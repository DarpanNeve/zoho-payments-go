package zoho

import (
	"errors"
	"net/http"
	"time"
)

const (
	baseIndiaProd    = "https://payments.zoho.in/api/v1"
	baseIndiaSandbox = "https://paymentssandbox.zoho.in/api/v1"
	baseGlobal       = "https://payments.zoho.com/api/v1"

	tokenIndia  = "https://accounts.zoho.in/oauth/v2/token"
	tokenGlobal = "https://accounts.zoho.com/oauth/v2/token"
)

type Region int

const (
	RegionIndia Region = iota
	RegionGlobal
)

type config struct {
	region        Region
	sandbox       bool
	baseOverride  string
	tokenOverride string
	httpClient    *http.Client
	maxRetries    int
}

func defaultConfig() config {
	return config{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 2,
	}
}

func (cfg config) resolve() (baseURL, tokenURL string, err error) {
	switch cfg.region {
	case RegionGlobal:
		baseURL, tokenURL = baseGlobal, tokenGlobal
		if cfg.sandbox && cfg.baseOverride == "" {
			return "", "", errors.New("zoho: sandbox is only documented for RegionIndia; use WithBaseURL for other regions")
		}
	default:
		baseURL, tokenURL = baseIndiaProd, tokenIndia
		if cfg.sandbox {
			baseURL = baseIndiaSandbox
		}
	}
	if cfg.baseOverride != "" {
		baseURL = cfg.baseOverride
	}
	if cfg.tokenOverride != "" {
		tokenURL = cfg.tokenOverride
	}
	return baseURL, tokenURL, nil
}

type Option func(*config)

func WithSandbox() Option {
	return func(c *config) { c.sandbox = true }
}

func WithRegion(r Region) Option {
	return func(c *config) { c.region = r }
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *config) {
		if h != nil {
			c.httpClient = h
		}
	}
}

func WithBaseURL(u string) Option {
	return func(c *config) { c.baseOverride = u }
}

func WithTokenURL(u string) Option {
	return func(c *config) { c.tokenOverride = u }
}

func WithMaxRetries(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}
