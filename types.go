package zoho

import (
	"strconv"
	"strings"
	"time"
)

// Amount tolerates Zoho's mixed encoding: requests use JSON numbers,
// responses return decimal strings ("100.50").
type Amount float64

func (a Amount) Float64() float64 { return float64(a) }

func (a *Amount) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*a = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return &DecodeError{Field: "amount", Value: s}
	}
	*a = Amount(f)
	return nil
}

func (a Amount) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(a), 'f', -1, 64)), nil
}

// Time tolerates epoch milliseconds (number or string) and RFC3339.
type Time struct {
	time.Time
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" || s == "0" {
		t.Time = time.Time{}
		return nil
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		t.Time = time.UnixMilli(ms)
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, s); err == nil {
		t.Time = parsed
		return nil
	}
	return &DecodeError{Field: "time", Value: s}
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(t.UnixMilli(), 10)), nil
}
