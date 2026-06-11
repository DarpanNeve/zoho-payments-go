package zoho

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func signBody(t *testing.T, body []byte, key string, ts int64) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + string(body)))
	return fmt.Sprintf("t=%d,v=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"event_type":"payment_link.paid"}`)
	key := "test-signing-key"
	ts := time.Now().UnixMilli()
	header := signBody(t, body, key, ts)

	if !VerifySignature(body, header, key) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature(body, header, "wrong-key") {
		t.Fatal("wrong key accepted")
	}
	if VerifySignature([]byte(`tampered`), header, key) {
		t.Fatal("tampered body accepted")
	}
	if VerifySignature(body, "", key) {
		t.Fatal("empty header accepted")
	}
	if VerifySignature(body, "t=123", key) {
		t.Fatal("missing v accepted")
	}
	if VerifySignature(body, header, "") {
		t.Fatal("empty signing key accepted")
	}
}

func TestVerifySignatureWithTolerance(t *testing.T) {
	body := []byte(`{"event_type":"payment.succeeded"}`)
	key := "k"

	fresh := signBody(t, body, key, time.Now().UnixMilli())
	if !VerifySignatureWithTolerance(body, fresh, key, 5*time.Minute) {
		t.Fatal("fresh signature rejected")
	}

	stale := signBody(t, body, key, time.Now().Add(-10*time.Minute).UnixMilli())
	if VerifySignatureWithTolerance(body, stale, key, 5*time.Minute) {
		t.Fatal("stale signature accepted")
	}
}

func TestParseEventPaymentLinkPaid(t *testing.T) {
	body := []byte(`{
		"event_id": 8210000000123,
		"event_type": "payment_link.paid",
		"account_id": 8210000000001,
		"live_mode": true,
		"event_time": 1780309800000,
		"event_object": {
			"payment_links": {
				"payment_link_id": "pl_001",
				"reference_id": "665f1c2a9d3e4b0012345678",
				"status": "paid",
				"amount": "4999.00",
				"payments": [{"payment_id": "pay_777"}]
			}
		}
	}`)

	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.EventType != EventPaymentLinkPaid {
		t.Fatalf("event type %q", ev.EventType)
	}
	if ev.EventID.String() != "8210000000123" {
		t.Fatalf("event id %q", ev.EventID)
	}
	if !ev.LiveMode {
		t.Fatal("live_mode not parsed")
	}

	obj, err := ev.PaymentLinkObject()
	if err != nil {
		t.Fatal(err)
	}
	pl := obj.PaymentLinks
	if pl.ReferenceID != "665f1c2a9d3e4b0012345678" {
		t.Fatalf("reference id %q", pl.ReferenceID)
	}
	if pl.Amount.Float64() != 4999 {
		t.Fatalf("amount %v", pl.Amount)
	}
	if pl.FirstPaymentID() != "pay_777" {
		t.Fatalf("payment id %q", pl.FirstPaymentID())
	}
}

func TestParseEventPaymentSucceeded(t *testing.T) {
	body := []byte(`{
		"event_type": "payment.succeeded",
		"event_object": {
			"payment": {
				"payment_id": "pay_777",
				"payment_link_id": "pl_001",
				"amount": "4999.00",
				"status": "succeeded",
				"phone": "9876543210",
				"date": 1780309800000
			}
		}
	}`)

	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ev.PaymentObject()
	if err != nil {
		t.Fatal(err)
	}
	p := obj.Payment
	if p.PaymentLinkID != "pl_001" || p.PaymentID != "pay_777" || p.Phone != "9876543210" {
		t.Fatalf("unexpected payment %+v", p)
	}
	if p.Date.IsZero() {
		t.Fatal("date not parsed")
	}
}

func TestParseEventRefundSucceeded(t *testing.T) {
	body := []byte(`{
		"event_type": "refund.succeeded",
		"event_object": {
			"refund": {
				"refund_id": "ref_42",
				"payment_id": "pay_777",
				"amount": "1000.00",
				"status": "succeeded",
				"date": "1780309800000"
			}
		}
	}`)

	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ev.RefundObject()
	if err != nil {
		t.Fatal(err)
	}
	if obj.Refund.RefundID != "ref_42" || obj.Refund.Amount.Float64() != 1000 {
		t.Fatalf("unexpected refund %+v", obj.Refund)
	}
}

func TestParseEventInvalid(t *testing.T) {
	if _, err := ParseEvent([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if _, err := ParseEvent([]byte(`{"foo":1}`)); err == nil {
		t.Fatal("expected error for missing event_type")
	}
}
