package zoho

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fixture struct {
	client     *Client
	tokenCalls *int32
	apiCalls   *int32
}

func newFixture(t *testing.T, apiHandler http.HandlerFunc) *fixture {
	t.Helper()
	var tokenCalls, apiCalls int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.FormValue("grant_type"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok-1", "expires_in": 3600})
	}))
	t.Cleanup(tokenSrv.Close)

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		if r.URL.Query().Get("account_id") != "acc-1" {
			t.Errorf("missing account_id, got query %q", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Zoho-oauthtoken tok-1" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		apiHandler(w, r)
	}))
	t.Cleanup(apiSrv.Close)

	c, err := New("acc-1", "cid", "csecret", "rtoken",
		WithBaseURL(apiSrv.URL), WithTokenURL(tokenSrv.URL), WithMaxRetries(2))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{client: c, tokenCalls: &tokenCalls, apiCalls: &apiCalls}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("", "a", "b", "c"); err == nil {
		t.Fatal("expected error for empty accountID")
	}
	if _, err := New("a", "b", "c", "d", WithRegion(RegionGlobal), WithSandbox()); err == nil {
		t.Fatal("expected error for global sandbox without base override")
	}
	if _, err := New("a", "b", "c", "d", WithSandbox()); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePaymentLinkRealResponseShape(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/paymentlinks" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var req CreatePaymentLinkRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Currency != "INR" {
			t.Errorf("currency = %q", req.Currency)
		}
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "Payment link created successfully",
			"payment_links": {
				"payment_link_id": "pl_100",
				"url": "https://payments.zoho.in/pl/pl_100",
				"amount": "4999.00",
				"amount_paid": "0.00",
				"status": "active",
				"reference_id": "booking-1",
				"expires_at": "2026-07-11",
				"created_time": 1780309800000
			}
		}`))
	})

	pl, err := f.client.CreatePaymentLink(context.Background(), CreatePaymentLinkRequest{
		Amount:           4999,
		Description:      "Trek booking",
		ReferenceID:      "booking-1",
		Phone:            "9876543210",
		PhoneCountryCode: "IN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pl.PaymentLinkID != "pl_100" || pl.URL == "" {
		t.Fatalf("unexpected link %+v", pl)
	}
	if pl.Amount.Float64() != 4999 {
		t.Fatalf("amount %v", pl.Amount)
	}
	if pl.CreatedTime.IsZero() {
		t.Fatal("created_time not parsed from epoch ms")
	}
	if pl.ExpiresAt != "2026-07-11" {
		t.Fatalf("expires_at %q", pl.ExpiresAt)
	}
}

func TestCreatePaymentLinkValidation(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be hit on validation failure")
	})
	ctx := context.Background()

	if _, err := f.client.CreatePaymentLink(ctx, CreatePaymentLinkRequest{Amount: 0, Description: "x"}); err == nil {
		t.Fatal("zero amount accepted")
	}
	if _, err := f.client.CreatePaymentLink(ctx, CreatePaymentLinkRequest{Amount: -5, Description: "x"}); err == nil {
		t.Fatal("negative amount accepted")
	}
	if _, err := f.client.CreatePaymentLink(ctx, CreatePaymentLinkRequest{Amount: 100}); err == nil {
		t.Fatal("empty description accepted")
	}
}

func TestCancelPaymentLinkUsesPutCancel(t *testing.T) {
	var gotMethod, gotPath string
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"code":0,"payment_links":{"payment_link_id":"pl_100","status":"canceled"}}`))
	})

	if err := f.client.CancelPaymentLink(context.Background(), "pl_100"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/paymentlinks/pl_100/cancel" {
		t.Fatalf("got %s %s, want PUT /paymentlinks/pl_100/cancel", gotMethod, gotPath)
	}
	if err := f.client.CancelPaymentLink(context.Background(), ""); err != nil {
		t.Fatal("empty link id should be a no-op")
	}
}

func TestUnauthorizedRetriesOnceWithFreshToken(t *testing.T) {
	var calls int32
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"error","message":"invalid oauth token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"payment":{"payment_id":"pay_1","status":"succeeded","amount":"10.00"}}`))
	})

	p, err := f.client.GetPayment(context.Background(), "pay_1")
	if err != nil {
		t.Fatal(err)
	}
	if p.PaymentID != "pay_1" {
		t.Fatalf("payment %+v", p)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("api calls = %d, want 2", calls)
	}
	if atomic.LoadInt32(f.tokenCalls) != 2 {
		t.Fatalf("token calls = %d, want 2 (initial + post-401 refresh)", *f.tokenCalls)
	}
}

func TestGetRetriesOn429(t *testing.T) {
	var calls int32
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"error","message":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"payment":{"payment_id":"pay_2","status":"succeeded"}}`))
	})

	p, err := f.client.GetPayment(context.Background(), "pay_2")
	if err != nil {
		t.Fatal(err)
	}
	if p.PaymentID != "pay_2" || atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("payment %+v calls %d", p, calls)
	}
}

func TestPostDoesNotRetryOn5xx(t *testing.T) {
	var calls int32
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"error","message":"upstream"}`))
	})

	_, err := f.client.CreatePaymentLink(context.Background(), CreatePaymentLinkRequest{Amount: 10, Description: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("POST retried: calls = %d", calls)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenCachedAcrossCalls(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"payment":{"payment_id":"p","status":"succeeded"}}`))
	})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := f.client.GetPayment(ctx, "p"); err != nil {
			t.Fatal(err)
		}
	}
	if atomic.LoadInt32(f.tokenCalls) != 1 {
		t.Fatalf("token calls = %d, want 1", *f.tokenCalls)
	}
}

func TestCreateRefundValidationAndPath(t *testing.T) {
	var gotPath string
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"code":0,"refund":{"refund_id":"ref_1","payment_id":"pay_1","amount":"100.00","status":"initiated","date":1780309800000}}`))
	})
	ctx := context.Background()

	if _, err := f.client.CreateRefund(ctx, "pay_1", CreateRefundRequest{Amount: 100}); err == nil {
		t.Fatal("missing reason/type accepted")
	}

	ref, err := f.client.CreateRefund(ctx, "pay_1", CreateRefundRequest{
		Amount: 100, Reason: RefundReasonRequestedByCustomer, Type: RefundTypeMerchant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/payments/pay_1/refunds" {
		t.Fatalf("path %q", gotPath)
	}
	if ref.RefundID != "ref_1" || ref.Amount.Float64() != 100 {
		t.Fatalf("refund %+v", ref)
	}
}

func TestGetRefundPath(t *testing.T) {
	var gotPath string
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"code":0,"refund":{"refund_id":"ref_9","status":"succeeded"}}`))
	})
	if _, err := f.client.GetRefund(context.Background(), "ref_9"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/refunds/ref_9" {
		t.Fatalf("path %q, want /refunds/ref_9", gotPath)
	}
}

func TestListPaymentsCustomDateValidation(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be hit")
	})
	_, err := f.client.ListPayments(context.Background(), ListPaymentsParams{FilterBy: FilterCustomDate})
	if err == nil {
		t.Fatal("CustomDate without from/to accepted")
	}
}
