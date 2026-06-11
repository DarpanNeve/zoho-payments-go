package zoho

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	userAgent       = "zoho-payments-go/1.0"
	maxResponseSize = 10 << 20
	tokenSafety     = 60 * time.Second
)

type Client struct {
	accountID    string
	clientID     string
	clientSecret string
	refreshToken string
	baseURL      string
	tokenURL     string
	httpClient   *http.Client
	maxRetries   int

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func New(accountID, clientID, clientSecret, refreshToken string, opts ...Option) (*Client, error) {
	if accountID == "" || clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, errors.New("zoho: accountID, clientID, clientSecret and refreshToken are all required")
	}

	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	baseURL, tokenURL, err := cfg.resolve()
	if err != nil {
		return nil, err
	}

	return &Client{
		accountID:    accountID,
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		baseURL:      baseURL,
		tokenURL:     tokenURL,
		httpClient:   cfg.httpClient,
		maxRetries:   cfg.maxRetries,
	}, nil
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("zoho: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zoho: token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("zoho: token read: %w", err)
	}

	var tok struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("zoho: token decode (http %d): %w", resp.StatusCode, err)
	}
	if tok.Error != "" {
		msg := tok.Error
		if tok.ErrorDescription != "" {
			msg += ": " + tok.ErrorDescription
		}
		return "", fmt.Errorf("zoho: token error: %s", msg)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("zoho: empty access token (http %d)", resp.StatusCode)
	}

	ttl := time.Duration(tok.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	if ttl > 2*tokenSafety {
		ttl -= tokenSafety
	} else {
		ttl /= 2
	}
	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(ttl)
	return c.accessToken, nil
}

func (c *Client) invalidateToken(used string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken == used {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body interface{}) ([]byte, int, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("zoho: marshal: %w", err)
		}
		payload = b
	}

	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, 0, fmt.Errorf("zoho: parse url: %w", err)
	}
	q := u.Query()
	q.Set("account_id", c.accountID)
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()

	retriedAuth := false
	retryable := method == http.MethodGet
	attempt := 0
	for {
		token, err := c.token(ctx)
		if err != nil {
			return nil, 0, err
		}

		respBody, status, err := c.send(ctx, method, u.String(), token, payload)
		if err != nil {
			if retryable && attempt < c.maxRetries {
				if werr := sleepBackoff(ctx, attempt); werr != nil {
					return nil, 0, werr
				}
				attempt++
				continue
			}
			return nil, 0, err
		}

		if status == http.StatusUnauthorized && !retriedAuth {
			c.invalidateToken(token)
			retriedAuth = true
			continue
		}
		if (status == http.StatusTooManyRequests || status >= 500) && retryable && attempt < c.maxRetries {
			if werr := sleepBackoff(ctx, attempt); werr != nil {
				return nil, 0, werr
			}
			attempt++
			continue
		}
		return respBody, status, nil
	}
}

func (c *Client) send(ctx context.Context, method, fullURL, token string, payload []byte) ([]byte, int, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("zoho: new request: %w", err)
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	req.Header.Set("User-Agent", userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("zoho: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("zoho: read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func sleepBackoff(ctx context.Context, attempt int) error {
	d := 400 * time.Millisecond << attempt
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
