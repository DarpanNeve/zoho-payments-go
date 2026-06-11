package zoho

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	EventPaymentSucceeded     = "payment.succeeded"
	EventPaymentFailed        = "payment.failed"
	EventPaymentLinkPaid      = "payment_link.paid"
	EventPaymentLinkExpired   = "payment_link.expired"
	EventPaymentLinkCanceled  = "payment_link.canceled"
	EventRefundSucceeded      = "refund.succeeded"
	EventRefundFailed         = "refund.failed"
	EventVirtualAccountPaid   = "virtual_account.paid"
	EventVirtualAccountClosed = "virtual_account.closed"
	EventPayoutInitiated      = "payout.initiated"
	EventPayoutPaid           = "payout.paid"
	EventPayoutFailed         = "payout.failed"

	SignatureHeader = "X-Zoho-Webhook-Signature"
)

type Event struct {
	EventID     json.Number     `json:"event_id"`
	EventType   string          `json:"event_type"`
	AccountID   json.Number     `json:"account_id"`
	LiveMode    bool            `json:"live_mode"`
	EventTime   Time            `json:"event_time"`
	EventObject json.RawMessage `json:"event_object"`
}

type WebhookPaymentLink struct {
	PaymentLinkID string        `json:"payment_link_id"`
	ReferenceID   string        `json:"reference_id"`
	Status        string        `json:"status"`
	Amount        Amount        `json:"amount"`
	AmountPaid    Amount        `json:"amount_paid"`
	Currency      string        `json:"currency"`
	Description   string        `json:"description"`
	Phone         string        `json:"phone"`
	Email         string        `json:"email"`
	Payments      []LinkPayment `json:"payments"`
}

func (w *WebhookPaymentLink) FirstPaymentID() string {
	if len(w.Payments) > 0 {
		return w.Payments[0].PaymentID
	}
	return ""
}

type PaymentLinkEventObject struct {
	PaymentLinks WebhookPaymentLink `json:"payment_links"`
}

type WebhookPayment struct {
	PaymentID       string `json:"payment_id"`
	PaymentLinkID   string `json:"payment_link_id"`
	Amount          Amount `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
	ReferenceNumber string `json:"reference_number"`
	Date            Time   `json:"date"`
	FailureReason   string `json:"failure_reason"`
}

type PaymentEventObject struct {
	Payment WebhookPayment `json:"payment"`
}

type RefundEventObject struct {
	Refund Refund `json:"refund"`
}

func VerifySignature(body []byte, sigHeader, signingKey string) bool {
	timestamp, signature, ok := parseSignatureHeader(sigHeader)
	if !ok || signingKey == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(timestamp + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(signature)))
}

func VerifySignatureWithTolerance(body []byte, sigHeader, signingKey string, tolerance time.Duration) bool {
	if !VerifySignature(body, sigHeader, signingKey) {
		return false
	}
	timestamp, _, _ := parseSignatureHeader(sigHeader)
	ms, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	drift := time.Since(time.UnixMilli(ms))
	if drift < 0 {
		drift = -drift
	}
	return drift <= tolerance
}

func parseSignatureHeader(header string) (timestamp, signature string, ok bool) {
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v":
			signature = kv[1]
		}
	}
	return timestamp, signature, timestamp != "" && signature != ""
}

func ParseEvent(body []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("zoho: parse event: %w", err)
	}
	if e.EventType == "" {
		return nil, fmt.Errorf("zoho: event missing event_type")
	}
	return &e, nil
}

func (e *Event) PaymentLinkObject() (*PaymentLinkEventObject, error) {
	var obj PaymentLinkEventObject
	if err := json.Unmarshal(e.EventObject, &obj); err != nil {
		return nil, fmt.Errorf("zoho: decode %s object: %w", e.EventType, err)
	}
	return &obj, nil
}

func (e *Event) PaymentObject() (*PaymentEventObject, error) {
	var obj PaymentEventObject
	if err := json.Unmarshal(e.EventObject, &obj); err != nil {
		return nil, fmt.Errorf("zoho: decode %s object: %w", e.EventType, err)
	}
	return &obj, nil
}

func (e *Event) RefundObject() (*RefundEventObject, error) {
	var obj RefundEventObject
	if err := json.Unmarshal(e.EventObject, &obj); err != nil {
		return nil, fmt.Errorf("zoho: decode %s object: %w", e.EventType, err)
	}
	return &obj, nil
}
