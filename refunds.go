package zoho

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	RefundReasonDuplicate           = "duplicate"
	RefundReasonFraudulent          = "fraudulent"
	RefundReasonRequestedByCustomer = "requested_by_customer"
	RefundReasonOthers              = "others"
	RefundReasonSystemInitiated     = "system_initiated"

	RefundTypeMerchant = "initiated_by_merchant"
	RefundTypeCustomer = "initiated_by_customer"
	RefundTypeSystem   = "initiated_by_system"

	RefundStatusInitiated = "initiated"
	RefundStatusSucceeded = "succeeded"
	RefundStatusFailed    = "failed"
	RefundStatusCanceled  = "canceled"
	RefundStatusPending   = "pending"
)

type CreateRefundRequest struct {
	Amount      float64 `json:"amount"`
	Reason      string  `json:"reason"`
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
}

func (r *CreateRefundRequest) validate() error {
	if r.Amount <= 0 {
		return &ValidationError{Field: "amount", Message: "must be greater than zero"}
	}
	if r.Reason == "" {
		return &ValidationError{Field: "reason", Message: "is required"}
	}
	if r.Type == "" {
		return &ValidationError{Field: "type", Message: "is required"}
	}
	return nil
}

type Refund struct {
	RefundID        string `json:"refund_id"`
	PaymentID       string `json:"payment_id"`
	ReferenceNumber string `json:"reference_number"`
	Amount          Amount `json:"amount"`
	Type            string `json:"type"`
	Reason          string `json:"reason"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	FailureReason   string `json:"failure_reason"`
	Date            Time   `json:"date"`
}

type refundEnvelope struct {
	Refund Refund `json:"refund"`
}

func (c *Client) CreateRefund(ctx context.Context, paymentID string, req CreateRefundRequest) (*Refund, error) {
	if paymentID == "" {
		return nil, errors.New("zoho: payment id is required")
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	body, status, err := c.do(ctx, http.MethodPost, "/payments/"+url.PathEscape(paymentID)+"/refunds", nil, req)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out refundEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode refund: %w", err)
	}
	if out.Refund.RefundID == "" {
		return nil, errors.New("zoho: refund response missing refund_id")
	}
	return &out.Refund, nil
}

func (c *Client) GetRefund(ctx context.Context, refundID string) (*Refund, error) {
	if refundID == "" {
		return nil, errors.New("zoho: refund id is required")
	}
	body, status, err := c.do(ctx, http.MethodGet, "/refunds/"+url.PathEscape(refundID), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out refundEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode refund: %w", err)
	}
	return &out.Refund, nil
}
