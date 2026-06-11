# zoho-payments-go

> **Unofficial SDK** — Not affiliated with or endorsed by Zoho Corporation. "Zoho" and "Zoho Payments" are trademarks of Zoho Corporation Pvt. Ltd.

Go client for the [Zoho Payments](https://www.zoho.com/in/payments/) API. Handles OAuth token refresh, payment links, payments, refunds, and webhook verification. No dependencies beyond the standard library.

```
go get github.com/darpanneve/zoho-payments-go
```

## Requirements

- Go 1.22+
- Zoho Payments account (India data center)
- OAuth credentials (Client ID, Client Secret, Refresh Token) — see [Getting credentials](#getting-credentials)

## Quick start

```go
import zoho "github.com/darpanneve/zoho-payments-go"

client, err := zoho.New(
    os.Getenv("ZOHO_ACCOUNT_ID"),
    os.Getenv("ZOHO_CLIENT_ID"),
    os.Getenv("ZOHO_CLIENT_SECRET"),
    os.Getenv("ZOHO_REFRESH_TOKEN"),
)
if err != nil {
    log.Fatal(err)
}

link, err := client.CreatePaymentLink(ctx, zoho.CreatePaymentLinkRequest{
    Amount:      4999.00, // rupees, not paise
    Description: zoho.SanitizeText("Kedarkantha Trek"),
    ReferenceID: bookingID,
    Phone:       zoho.NormalizePhone("919876543210", "IN"),
    PhoneCountryCode: "IN",
})
```

Create **one** client per process and reuse it — it caches the access token and is safe for concurrent use.

## Getting credentials

**1. Register a Server-based Application** at [api-console.zoho.in](https://api-console.zoho.in). Use "Server-based Application", not "Self Client" — the self-client flow lacks a redirect URI and fails for Zoho Payments.

**2. Authorize in a browser** (logged in as the account owner):

```
https://accounts.zoho.in/oauth/v2/auth
  ?response_type=code
  &client_id=YOUR_CLIENT_ID
  &scope=ZohoPay.payments.CREATE,ZohoPay.payments.READ,ZohoPay.payments.UPDATE
  &redirect_uri=YOUR_REDIRECT_URI
  &access_type=offline
  &soid=zohopay.YOUR_ACCOUNT_ID
```

The scope prefix is `ZohoPay.` (not `ZohoPayments.*`). The `soid` parameter is required — omitting it causes silent auth failures. The authorization code expires in 60 seconds.

**3. Exchange the code:**

```bash
curl -X POST "https://accounts.zoho.in/oauth/v2/token" \
  -d "grant_type=authorization_code" \
  -d "client_id=YOUR_CLIENT_ID" \
  -d "client_secret=YOUR_CLIENT_SECRET" \
  -d "redirect_uri=YOUR_REDIRECT_URI" \
  -d "code=PASTE_CODE_HERE"
```

Save the `refresh_token`. The SDK handles all subsequent token refreshes automatically.

## Client options

```go
zoho.New(acc, id, secret, refresh,
    zoho.WithSandbox(),                              // paymentssandbox.zoho.in
    zoho.WithRegion(zoho.RegionGlobal),              // payments.zoho.com
    zoho.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
    zoho.WithMaxRetries(3),
    zoho.WithBaseURL("https://..."),                 // override for testing
    zoho.WithTokenURL("https://..."),
)
```

`WithSandbox()` only works with the India region. Pairing it with `RegionGlobal` returns an error unless you also provide `WithBaseURL`.

## Payment links

```go
// Create
link, err := client.CreatePaymentLink(ctx, zoho.CreatePaymentLinkRequest{
    Amount:           4999.00,
    Description:      zoho.SanitizeText("Trek booking"),
    ReferenceID:      bookingID,        // comes back in webhooks
    Phone:            "9876543210",
    PhoneCountryCode: "IN",
    Email:            "user@example.com",
    ExpiresAt:        "2026-07-15",     // yyyy-MM-dd
})
// link.URL, link.PaymentLinkID

// Get
link, err = client.GetPaymentLink(ctx, linkID)
if link.IsPaid() { ... }

// Cancel
err = client.CancelPaymentLink(ctx, linkID)

// Update
link, err = client.UpdatePaymentLink(ctx, linkID, zoho.UpdatePaymentLinkRequest{
    ExpiresAt: "2026-08-01",
})
```

`Amount` is in **rupees** (Zoho Payments), not paise (Razorpay). `CancelPaymentLink` with an empty ID is a silent no-op.

## Payments

```go
p, err := client.GetPayment(ctx, paymentID)
fmt.Println(p.Amount.Float64(), p.Status)

payments, err := client.ListPayments(ctx, zoho.ListPaymentsParams{
    Status:   zoho.ListStatusSucceeded,
    FilterBy: zoho.FilterLast30Days,
    PerPage:  100,
})

// custom date range
payments, err = client.ListPayments(ctx, zoho.ListPaymentsParams{
    FilterBy: zoho.FilterCustomDate,
    FromDate: "2026-06-01",
    ToDate:   "2026-06-11",
})
```

## Refunds

`Reason` and `Type` are required by the API. The client validates locally so you catch missing fields in development.

```go
refund, err := client.CreateRefund(ctx, paymentID, zoho.CreateRefundRequest{
    Amount: 1000.00,
    Reason: zoho.RefundReasonRequestedByCustomer,
    Type:   zoho.RefundTypeMerchant,
})

refund, err = client.GetRefund(ctx, refund.RefundID)
// refund.Status: initiated | succeeded | failed
```

Refunds are async — `CreateRefund` returns `initiated`. Track via the `refund.succeeded` / `refund.failed` webhook or poll `GetRefund`.

## Webhooks

```go
func handler(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))

    sig := r.Header.Get(zoho.SignatureHeader)
    if !zoho.VerifySignatureWithTolerance(body, sig, os.Getenv("ZOHO_SIGNING_KEY"), 5*time.Minute) {
        http.Error(w, "Unauthorized", 401)
        return
    }

    ev, err := zoho.ParseEvent(body)
    if err != nil {
        w.WriteHeader(http.StatusOK) // malformed but authentic — ack to stop retries
        return
    }

    w.WriteHeader(http.StatusOK) // ack before processing
    _, _ = w.Write([]byte("OK"))
    if f, ok := w.(http.Flusher); ok { f.Flush() }

    switch ev.EventType {
    case zoho.EventPaymentLinkPaid:
        obj, _ := ev.PaymentLinkObject()
        // obj.PaymentLinks.ReferenceID
    case zoho.EventPaymentSucceeded:
        obj, _ := ev.PaymentObject()
        // obj.Payment.PaymentLinkID
    case zoho.EventRefundSucceeded, zoho.EventRefundFailed:
        obj, _ := ev.RefundObject()
        // obj.Refund.Status
    }
}
```

Verify before anything else. Both `payment_link.paid` and `payment.succeeded` fire per successful payment — make your "mark paid" writes idempotent.

**Event constants:** `EventPaymentSucceeded`, `EventPaymentFailed`, `EventPaymentLinkPaid`, `EventPaymentLinkExpired`, `EventPaymentLinkCanceled`, `EventRefundSucceeded`, `EventRefundFailed`, `EventVirtualAccountPaid`, `EventVirtualAccountClosed`, `EventPayoutInitiated`, `EventPayoutPaid`, `EventPayoutFailed`.

## Error handling

```go
var apiErr *zoho.APIError
if errors.As(err, &apiErr) {
    switch {
    case apiErr.IsRateLimited(): // 429
    case apiErr.IsAuthError():   // 401/403
    case apiErr.IsNotFound():    // 404
    default:
        log.Printf("code=%d http=%d msg=%s", apiErr.Code, apiErr.HTTPStatus, apiErr.Message)
    }
}
```

## Retry policy

| Trigger | Behavior |
|---|---|
| `401` | Token invalidated, refreshed, request retried once (any method) |
| `429` / `5xx` on GET | Exponential backoff (400ms · 2ⁿ), up to `WithMaxRetries` (default 2) |
| `429` / `5xx` on POST/PUT | Not retried — no idempotency keys, retrying creates could duplicate records |

## Helpers

```go
zoho.SanitizeText("Trek <Booking>!")   // strips chars Zoho rejects
zoho.NormalizePhone("919876543210", "IN") // → "9876543210"
amount.Float64()                       // zoho.Amount → float64
link.CreatedTime.Time                  // zoho.Time embeds time.Time
```

## Environment variables

```
ZOHO_ACCOUNT_ID
ZOHO_CLIENT_ID
ZOHO_CLIENT_SECRET
ZOHO_REFRESH_TOKEN
ZOHO_SIGNING_KEY    # for webhook verification
ZOHO_SANDBOX=true   # optional — enables sandbox environment
```

## Testing

```bash
go test ./...
go test -race ./...
```

Tests run entirely offline against fake token and API servers via `httptest`. To point the client at your own test server:

```go
client, _ := zoho.New("acc", "id", "secret", "refresh",
    zoho.WithBaseURL(mockSrv.URL),
    zoho.WithTokenURL(mockToken.URL),
)
```

## License

MIT
