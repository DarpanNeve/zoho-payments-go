package zoho

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type APIError struct {
	Code       int
	CodeText   string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	code := strconv.Itoa(e.Code)
	if e.CodeText != "" {
		code = e.CodeText
	}
	return fmt.Sprintf("zoho: api error %s (http %d): %s", code, e.HTTPStatus, e.Message)
}

func (e *APIError) IsRateLimited() bool { return e.HTTPStatus == http.StatusTooManyRequests }
func (e *APIError) IsNotFound() bool    { return e.HTTPStatus == http.StatusNotFound }
func (e *APIError) IsAuthError() bool {
	return e.HTTPStatus == http.StatusUnauthorized || e.HTTPStatus == http.StatusForbidden
}

type DecodeError struct {
	Field string
	Value string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("zoho: cannot decode %s value %q", e.Field, e.Value)
}

// flexCode tolerates Zoho returning "code" as 0 (success), an int, or a
// string like "error" depending on the failure path.
type flexCode struct {
	num  int
	text string
	set  bool
}

func (f *flexCode) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		return nil
	}
	s = strings.Trim(s, `"`)
	if n, err := strconv.Atoi(s); err == nil {
		f.num = n
		f.set = true
		return nil
	}
	f.text = s
	f.set = true
	return nil
}

type envelope struct {
	Code    flexCode `json:"code"`
	Message string   `json:"message"`
}

func checkResp(body []byte, status int) error {
	var e envelope
	_ = json.Unmarshal(body, &e)

	failed := status >= 400 || e.Code.text != "" || (e.Code.set && e.Code.num != 0)
	if !failed {
		return nil
	}

	msg := e.Message
	if msg == "" {
		msg = truncateString(string(body), 256)
	}
	return &APIError{Code: e.Code.num, CodeText: e.Code.text, Message: msg, HTTPStatus: status}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
