package zoho

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	PaymentStatusInitiated         = "initiated"
	PaymentStatusSucceeded         = "succeeded"
	PaymentStatusFailed            = "failed"
	PaymentStatusCanceled          = "canceled"
	PaymentStatusIncomplete        = "incomplete"
	PaymentStatusRefunded          = "refunded"
	PaymentStatusPartiallyRefunded = "partially_refunded"
	PaymentStatusBlocked           = "blocked"
	PaymentStatusDisputed          = "disputed"

	ListStatusAll       = "Status.All"
	ListStatusSucceeded = "Status.Succeeded"
	ListStatusFailed    = "Status.Failed"
	ListStatusCancelled = "Status.Cancelled"
	ListStatusRefunded  = "Status.Refunded"
	ListStatusDisputed  = "Status.Disputed"

	FilterToday         = "ChargeDate.Today"
	FilterThisMonth     = "ChargeDate.ThisMonth"
	FilterThisYear      = "ChargeDate.ThisYear"
	FilterPreviousMonth = "ChargeDate.PreviousMonth"
	FilterPreviousYear  = "ChargeDate.PreviousYear"
	FilterLast30Days    = "ChargeDate.Last_30_Days"
	FilterCustomDate    = "ChargeDate.CustomDate"

	maxPerPage = 200
)

type PaymentMethod struct {
	Type string `json:"type"`
}

type Payment struct {
	PaymentID                  string        `json:"payment_id"`
	PaymentLinkID              string        `json:"payment_link_id"`
	Amount                     Amount        `json:"amount"`
	AmountCaptured             Amount        `json:"amount_captured"`
	AmountRefunded             Amount        `json:"amount_refunded"`
	FeeAmount                  Amount        `json:"fee_amount"`
	NetAmount                  Amount        `json:"net_amount"`
	Currency                   string        `json:"currency"`
	Status                     string        `json:"status"`
	Date                       Time          `json:"date"`
	Phone                      string        `json:"phone"`
	DialingCode                string        `json:"dialing_code"`
	ReceiptEmail               string        `json:"receipt_email"`
	CustomerID                 string        `json:"customer_id"`
	PaymentType                string        `json:"payment_type"`
	TransactionType            string        `json:"transaction_type"`
	ReferenceNumber            string        `json:"reference_number"`
	TransactionReferenceNumber string        `json:"transaction_reference_number"`
	InvoiceNumber              string        `json:"invoice_number"`
	PaymentMethod              PaymentMethod `json:"payment_method"`
}

type ListPaymentsParams struct {
	Status            string
	SearchText        string
	FilterBy          string
	FromDate          string
	ToDate            string
	PaymentMethodType string
	Page              int
	PerPage           int
}

func (p ListPaymentsParams) values() (url.Values, error) {
	if p.FilterBy == FilterCustomDate && (p.FromDate == "" || p.ToDate == "") {
		return nil, errors.New("zoho: from_date and to_date are required with ChargeDate.CustomDate")
	}
	q := url.Values{}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.SearchText != "" {
		q.Set("search_text", p.SearchText)
	}
	if p.FilterBy != "" {
		q.Set("filter_by", p.FilterBy)
	}
	if p.FromDate != "" {
		q.Set("from_date", p.FromDate)
	}
	if p.ToDate != "" {
		q.Set("to_date", p.ToDate)
	}
	if p.PaymentMethodType != "" {
		q.Set("payment_method_type", p.PaymentMethodType)
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.PerPage > 0 {
		if p.PerPage > maxPerPage {
			p.PerPage = maxPerPage
		}
		q.Set("per_page", strconv.Itoa(p.PerPage))
	}
	return q, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	if paymentID == "" {
		return nil, errors.New("zoho: payment id is required")
	}
	body, status, err := c.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out struct {
		Payment Payment `json:"payment"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode payment: %w", err)
	}
	return &out.Payment, nil
}

func (c *Client) ListPayments(ctx context.Context, params ListPaymentsParams) ([]Payment, error) {
	q, err := params.values()
	if err != nil {
		return nil, err
	}

	body, status, err := c.do(ctx, http.MethodGet, "/payments", q, nil)
	if err != nil {
		return nil, err
	}
	if err := checkResp(body, status); err != nil {
		return nil, err
	}

	var out struct {
		Payments []Payment `json:"payments"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("zoho: decode payments list: %w", err)
	}
	return out.Payments, nil
}
