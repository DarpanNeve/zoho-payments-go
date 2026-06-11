package zoho

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	PaymentLinkStatusActive   = "active"
	PaymentLinkStatusPaid     = "paid"
	PaymentLinkStatusCanceled = "canceled"
	PaymentLinkStatusExpired  = "expired"

	MethodCard         = "card"
	MethodUPI          = "upi"
	MethodNetBanking   = "netbanking"
	MethodWallet       = "wallet"
	MethodBankTransfer = "bank_transfer"

	maxDescriptionRunes = 500
	maxReferenceIDLen   = 100
)

type NotifyCustomer struct {
	Email bool `json:"email"`
	SMS   bool `json:"sms"`
}

type Configurations struct {
	AllowedPaymentMethods []string `json:"allowed_payment_methods,omitempty"`
}

type CreatePaymentLinkRequest struct {
	Amount           float64         `json:"amount"`
	Currency         string          `json:"currency"`
	Description      string          `json:"description"`
	ReferenceID      string          `json:"reference_id,omitempty"`
	Email            string          `json:"email,omitempty"`
	Phone            string          `json:"phone,omitempty"`
	PhoneCountryCode string          `json:"phone_country_code,omitempty"`
	ExpiresAt        string          `json:"expires_at,omitempty"`
	ReturnURL        string          `json:"return_url,omitempty"`
	NotifyCustomer   *NotifyCustomer `json:"notify_customer,omitempty"`
	Configurations   *Configurations `json:"configurations,omitempty"`
}

func (r *CreatePaymentLinkRequest) validate() error {
	if r.Amount <= 0 {
		return &ValidationError{Field: "amount", Message: "must be greater than zero"}
	}
	if strings.TrimSpace(r.Description) == "" {
		return &ValidationError{Field: "description", Message: "is required"}
	}
	if utf8.RuneCountInString(r.Description) > maxDescriptionRunes {
		return &ValidationError{Field: "description", Message: fmt.Sprintf("exceeds %d characters", maxDescriptionRunes)}
	}
	if len(r.ReferenceID) > maxReferenceIDLen {
		return &ValidationError{Field: "reference_id", Message: fmt.Sprintf("exceeds %d characters", maxReferenceIDLen)}
	}
	return nil
}

type UpdatePaymentLinkRequest struct {
	ReferenceID      string          `json:"reference_id,omitempty"`
	Email            string          `json:"email,omitempty"`
	Phone            string          `json:"phone,omitempty"`
	PhoneCountryCode string          `json:"phone_country_code,omitempty"`
	Description      string          `json:"description,omitempty"`
	ExpiresAt        string          `json:"expires_at,omitempty"`
	ReturnURL        string          `json:"return_url,omitempty"`
	NotifyCustomer   *NotifyCustomer `json:"notify_customer,omitempty"`
	Configurations   *Configurations `json:"configurations,omitempty"`
}

type LinkPayment struct {
	PaymentID string `json:"payment_id"`
}

type PaymentLink struct {
	PaymentLinkID    string        `json:"payment_link_id"`
	URL              string        `json:"url"`
	Amount           Amount        `json:"amount"`
	AmountPaid       Amount        `json:"amount_paid"`
	Currency         string        `json:"currency"`
	Status           string        `json:"status"`
	Description      string        `json:"description"`
	ReferenceID      string        `json:"reference_id"`
	Email            string        `json:"email"`
	Phone            string        `json:"phone"`
	PhoneCountryCode string        `json:"phone_country_code"`
	ExpiresAt        string        `json:"expires_at"`
	ReturnURL        string        `json:"return_url"`
	CreatedTime      Time          `json:"created_time"`
	Payments         []LinkPayment `json:"payments"`
}

func (p *PaymentLink) IsPaid() bool { return p.Status == PaymentLinkStatusPaid }

type paymentLinkEnvelope struct {
	PaymentLinks PaymentLink `json:"payment_links"`
}

func (c *Client) CreatePaymentLink(ctx context.Context, req CreatePaymentLinkRequest) (*PaymentLink, error) {
	if req.Currency == "" {
		req.Currency = "INR"
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	body, status, err := c.do(ctx, http.MethodPost, "/paymentlinks", nil, req)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out paymentLinkEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode payment link: %w", err)
	}
	if out.PaymentLinks.PaymentLinkID == "" || out.PaymentLinks.URL == "" {
		return nil, errors.New("zoho: payment link response missing id or url")
	}
	return &out.PaymentLinks, nil
}

func (c *Client) GetPaymentLink(ctx context.Context, linkID string) (*PaymentLink, error) {
	if linkID == "" {
		return nil, errors.New("zoho: payment link id is required")
	}
	body, status, err := c.do(ctx, http.MethodGet, "/paymentlinks/"+url.PathEscape(linkID), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out paymentLinkEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode payment link: %w", err)
	}
	return &out.PaymentLinks, nil
}

func (c *Client) UpdatePaymentLink(ctx context.Context, linkID string, req UpdatePaymentLinkRequest) (*PaymentLink, error) {
	if linkID == "" {
		return nil, errors.New("zoho: payment link id is required")
	}
	body, status, err := c.do(ctx, http.MethodPut, "/paymentlinks/"+url.PathEscape(linkID), nil, req)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out paymentLinkEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode payment link: %w", err)
	}
	return &out.PaymentLinks, nil
}

func (c *Client) CancelPaymentLink(ctx context.Context, linkID string) error {
	if linkID == "" {
		return nil
	}
	body, status, err := c.do(ctx, http.MethodPut, "/paymentlinks/"+url.PathEscape(linkID)+"/cancel", nil, nil)
	if err != nil {
		return err
	}
	return checkResp(body, status)
}
