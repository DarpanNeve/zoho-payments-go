# zoho-payments-go

> **Unofficial SDK** Not affiliated with or endorsed by Zoho Corporation. "Zoho" and "Zoho Payments" are trademarks of Zoho Corporation Pvt. Ltd.

Go client for the [Zoho Payments](https://syntexco.com/qr/zoho-payments) API. Covers payment links, payments, refunds, and webhooks. Zero dependencies standard library only.

```
go get github.com/darpanneve/zoho-payments-go
```

**Requires Go 1.22+**

## Getting credentials

You need four values: **Account ID**, **Client ID**, **Client Secret**, and **Refresh Token**.

**Account ID** : Zoho Payments dashboard → Settings → Account Details.

**Client ID + Client Secret** : [api-console.zoho.in](https://api-console.zoho.in) → Create → **Server-based Application**. Do not use "Self Client" : it has no redirect URI and fails for Zoho Payments. Any redirect URI works for the one-time flow (e.g. `https://example.com/callback`).

**Refresh Token** : one-time browser flow:

1. Open this URL while logged into the Zoho Payments account owner:

```
https://accounts.zoho.in/oauth/v2/auth
  ?response_type=code
  &client_id=YOUR_CLIENT_ID
  &scope=ZohoPay.payments.CREATE,ZohoPay.payments.READ,ZohoPay.payments.UPDATE
  &redirect_uri=YOUR_REDIRECT_URI
  &access_type=offline
  &soid=zohopay.YOUR_ACCOUNT_ID
```

Two things that trip people up:
- Scope prefix is `ZohoPay.` : not `ZohoPayments.*` (different product, causes 401s)
- `soid=zohopay.YOUR_ACCOUNT_ID` is required : missing it causes silent auth failures

2. After approving, copy the `code` from the redirect URL. It expires in **60 seconds**.

3. Exchange it:

```bash
curl -X POST "https://accounts.zoho.in/oauth/v2/token" \
  -d "grant_type=authorization_code" \
  -d "client_id=YOUR_CLIENT_ID" \
  -d "client_secret=YOUR_CLIENT_SECRET" \
  -d "redirect_uri=YOUR_REDIRECT_URI" \
  -d "code=PASTE_CODE_HERE"
```

Save the `refresh_token`. The SDK handles all subsequent token refreshes : you never touch tokens again.

**Sandbox** : email `support@zohopayments.com` with your Account ID to request sandbox access (~1 business day). Sandbox and production use **completely separate** OAuth apps, refresh tokens, signing keys, and account IDs.

## Client setup

```go
import zoho "github.com/darpanneve/zoho-payments-go"

client, err := zoho.New(
    os.Getenv("ZOHO_ACCOUNT_ID"),
    os.Getenv("ZOHO_CLIENT_ID"),
    os.Getenv("ZOHO_CLIENT_SECRET"),
    os.Getenv("ZOHO_REFRESH_TOKEN"),
    zoho.WithSigningKey(os.Getenv("ZOHO_WEBHOOK_SIGNING_KEY")),
)
if err != nil {
    log.Fatal(err)
}
```

`New` fails fast if any credential is empty. Create **one client per process** and share it : it is goroutine-safe and caches the access token internally.

Pass `WithSigningKey` at init time so the client can verify webhooks via `client.VerifyWebhook`. If `WithSigningKey` is omitted, the SDK falls back to the `ZOHO_WEBHOOK_SIGNING_KEY` environment variable.

### Options

```go
zoho.New(acc, id, secret, refresh,
    zoho.WithSigningKey("your-signing-key"),                    // webhook signature verification
    zoho.WithSandbox(),                                        // use sandbox environment (India only)
    zoho.WithRegion(zoho.RegionGlobal),                        // switch to payments.zoho.com
    zoho.WithHTTPClient(&http.Client{Timeout: 15 * time.Second}),
    zoho.WithMaxRetries(3),                                    // GET retries on 429/5xx (default: 2)
    zoho.WithBaseURL("https://..."),                           // override API base (for testing)
    zoho.WithTokenURL("https://..."),                          // override token endpoint (for testing)
)
```

`WithSandbox()` paired with `RegionGlobal` returns an error unless you also supply `WithBaseURL`.

Sandbox env vars:

```go
opts := []zoho.Option{}
if os.Getenv("ZOHO_SANDBOX") == "true" {
    opts = append(opts, zoho.WithSandbox())
}
client, err := zoho.New(accountID, clientID, clientSecret, refreshToken, opts...)
```

## Payment links

### Create

```go
link, err := client.CreatePaymentLink(ctx, zoho.CreatePaymentLinkRequest{
    Amount:           4999.00,                           // RUPEES : not paise
    Currency:         "INR",                             // optional, defaults to INR
    Description:      zoho.SanitizeText("Kedarkantha Trek : 2 guests"),
    ReferenceID:      bookingID,                         // your ID : returned in webhooks
    Phone:            zoho.NormalizePhone("919876543210", "IN"), // strips country prefix → "9876543210"
    PhoneCountryCode: "IN",
    Email:            "customer@example.com",            // optional
    ExpiresAt:        "2026-07-15",                      // optional, format: yyyy-MM-dd
    ReturnURL:        "https://yoursite.com/thanks",     // optional
})
if err != nil {
    // nothing was charged
}

// link.URL           : hosted checkout page to send to customer
// link.PaymentLinkID : store this for webhook lookup and cancellation
```

Validation runs before any network call: amount must be > 0, description is required (max 500 chars), reference ID max 100 chars.

**Phone numbers from WhatsApp** arrive as `919876543210` (12 digits). Zoho checkout pre-fill needs the bare 10-digit number + `PhoneCountryCode: "IN"`. `zoho.NormalizePhone` handles this.

**Description** : always run user input through `zoho.SanitizeText`. Zoho rejects many special characters and returns an error if unsanitized text slips through.

### Get

```go
link, err := client.GetPaymentLink(ctx, linkID)

// link.Status:   "active" | "paid" | "canceled" | "expired"
// link.IsPaid()  : convenience method

fmt.Println(link.Amount.Float64())       // zoho.Amount → float64
fmt.Println(link.CreatedTime.Time)       // zoho.Time   → time.Time
```

### Update

```go
link, err := client.UpdatePaymentLink(ctx, linkID, zoho.UpdatePaymentLinkRequest{
    ExpiresAt: "2026-08-01",
})
// only active links can be updated
```

### Cancel

```go
err := client.CancelPaymentLink(ctx, linkID)
// empty linkID is a no-op : safe to call without checking
```

## Payments

### Get

```go
p, err := client.GetPayment(ctx, paymentID)

// p.Status:  "initiated" | "succeeded" | "failed" | "canceled" |
//            "refunded" | "partially_refunded" | "disputed"
fmt.Println(p.Amount.Float64())
fmt.Println(p.FeeAmount.Float64())
fmt.Println(p.NetAmount.Float64())
```

### List

```go
payments, err := client.ListPayments(ctx, zoho.ListPaymentsParams{
    Status:   zoho.ListStatusSucceeded,   // "Status.Succeeded"
    FilterBy: zoho.FilterLast30Days,      // "ChargeDate.Last_30_Days"
    PerPage:  100,                        // max 200
    Page:     1,
})

// Custom date range:
payments, err = client.ListPayments(ctx, zoho.ListPaymentsParams{
    FilterBy: zoho.FilterCustomDate,
    FromDate: "2026-06-01",               // required when using CustomDate
    ToDate:   "2026-06-11",
})

// Search by phone, email, payment ID, or reference:
payments, err = client.ListPayments(ctx, zoho.ListPaymentsParams{
    SearchText: "9876543210",
})
```

## Refunds

`Reason` and `Type` are **required by the Zoho API**. The client validates them before making the request.

```go
refund, err := client.CreateRefund(ctx, paymentID, zoho.CreateRefundRequest{
    Amount:      1000.00,                                // partial refunds supported
    Reason:      zoho.RefundReasonRequestedByCustomer,   // or: Duplicate | Fraudulent | Others | SystemInitiated
    Type:        zoho.RefundTypeMerchant,                // or: RefundTypeCustomer | RefundTypeSystem
    Description: "Trek cancelled : weather",             // optional
})

// refund.Status: "initiated" (typical) : refunds are async
```

### Get refund

```go
refund, err := client.GetRefund(ctx, refundID)
// refund.Status:        "initiated" | "succeeded" | "failed" | "canceled" | "pending"
// refund.FailureReason  populated when Status == "failed"
```

Track completion via `refund.succeeded` / `refund.failed` webhooks, or poll `GetRefund`.

## Webhooks

Configure the endpoint in Zoho Payments → Developer Space → Webhooks. Copy the **Signing Key** when shown : it's only displayed once.

### Handler

```go
func zohoWebhookHandler(client *zoho.Client) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
        body, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, "Bad Request", http.StatusBadRequest)
            return
        }

        if err := client.VerifyWebhook(body, r.Header.Get(zoho.SignatureHeader)); err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

    ev, err := zoho.ParseEvent(body)
    if err != nil {
        w.WriteHeader(http.StatusOK) // authentic but unparseable : ack to stop retries
        return
    }

    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("OK"))
    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }

    ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
    defer cancel()

    switch ev.EventType {

    case zoho.EventPaymentLinkPaid:
        obj, err := ev.PaymentLinkObject()
        if err != nil {
            return
        }
        pl := obj.PaymentLinks
        // pl.ReferenceID  : the value you set at CreatePaymentLink
        // pl.FirstPaymentID() : the payment ID

    case zoho.EventPaymentSucceeded:
        obj, err := ev.PaymentObject()
        if err != nil {
            return
        }
        // NOTE: payment.succeeded does NOT carry reference_id.
        // Look up your record by obj.Payment.PaymentLinkID instead.

    case zoho.EventRefundSucceeded, zoho.EventRefundFailed:
        obj, _ := ev.RefundObject()
        // obj.Refund.RefundID, obj.Refund.Status

    }
    }
}
```

### Signature scheme

Header: `X-Zoho-Webhook-Signature: t=<unix-ms>,v=<hmac-sha256-hex>`

Signed string: `<timestamp> + "." + <raw body>`

`VerifySignature` does a constant-time comparison. `VerifySignatureWithTolerance` additionally rejects events older than the given window. Always verify against the **raw** body bytes : re-serializing the JSON breaks the HMAC.

### Deduplication

Both `payment_link.paid` and `payment.succeeded` fire for every successful payment. Your database update must be idempotent (e.g. a conditional update that only writes when `status != "paid"`), otherwise the customer receives two confirmation messages.

Use `ev.EventID` as a webhook-level dedup key. `ev.LiveMode` is `false` for sandbox deliveries.

### Event constants

```
EventPaymentSucceeded       EventPaymentFailed
EventPaymentLinkPaid        EventPaymentLinkExpired     EventPaymentLinkCanceled
EventRefundSucceeded        EventRefundFailed
EventVirtualAccountPaid     EventVirtualAccountClosed
EventPayoutInitiated        EventPayoutPaid             EventPayoutFailed
```

## Error handling

```go
link, err := client.CreatePaymentLink(ctx, req)
if err != nil {
    var apiErr *zoho.APIError
    if errors.As(err, &apiErr) {
        switch {
        case apiErr.IsRateLimited(): // HTTP 429
            scheduleRetry()
        case apiErr.IsAuthError():   // 401 or 403 : check scopes, soid, region
            alertOps(apiErr)
        case apiErr.IsNotFound():    // 404
            handleGone()
        default:
            log.Printf("zoho error code=%d http=%d: %s",
                apiErr.Code, apiErr.HTTPStatus, apiErr.Message)
        }
        return
    }
    // network failure, context cancelled, or decode error
}
```

`*zoho.ValidationError` is returned for locally-detected problems (missing fields, invalid amounts) before any network call. `*zoho.DecodeError` is returned when Zoho responds with an unexpected field encoding.

## Retry behavior

| Situation | What happens |
|---|---|
| `401 Unauthorized` | Token cache cleared, fresh token fetched, request retried **once** (any method) |
| `429` / `5xx` on **GET** | Exponential backoff : 400ms × 2ⁿ : up to `WithMaxRetries` attempts (default 2) |
| `429` / `5xx` on **POST / PUT** | **Not retried** : Zoho Payments has no idempotency keys; retrying creates could produce duplicate links or double charges |

Token cache: access tokens are refreshed ~60 seconds before expiry. Concurrent goroutines share a single in-flight refresh (no stampede).

Zoho's documented rate limits: 600 req/min for payments, 60 req/min for refunds.

## Helpers

```go
zoho.SanitizeText("Trek <Kedarkantha> & Stay!")
// → "Trek Kedarkantha  Stay"
// Strips characters Zoho rejects in description/name fields.
// Always run user input through this before passing to CreatePaymentLink.

zoho.NormalizePhone("919876543210", "IN")
// → "9876543210"
// Strips the country prefix from WhatsApp-style numbers (12-digit → 10-digit).
// Pass "IN" for India. Leaves 10-digit numbers unchanged.

amount.Float64()         // zoho.Amount → float64 (Zoho returns amounts as decimal strings)
link.CreatedTime.Time    // zoho.Time embeds time.Time (Zoho returns timestamps as epoch ms)
```

## Environment variables

```
ZOHO_ACCOUNT_ID      : dashboard → Settings → Account Details
ZOHO_CLIENT_ID       : api-console.zoho.in
ZOHO_CLIENT_SECRET   : api-console.zoho.in
ZOHO_REFRESH_TOKEN   : from the one-time OAuth flow above
ZOHO_WEBHOOK_SIGNING_KEY  : Developer Space → Webhooks → your endpoint → Signing Key
ZOHO_SANDBOX              : set to "true" to use sandbox environment
```

Never commit credentials. Use environment variables or a secrets manager.

## Testing

```bash
go test ./...
go test -race ./...
```

Tests run fully offline using `net/http/httptest` servers : no Zoho credentials required. The suite covers auth header injection, account ID propagation, 401 token retry, 429 backoff, POST non-retry, token caching, real Zoho response shapes (string amounts, epoch timestamps), and webhook signature verification.

To write your own tests against the SDK:

```go
apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    _, _ = w.Write([]byte(`{"code":0,"payment_links":{"payment_link_id":"pl_1","url":"https://...","amount":"4999.00","status":"active"}}`))
}))

tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    _ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-tok", "expires_in": 3600})
}))

client, _ := zoho.New("acc", "cid", "csecret", "rtoken",
    zoho.WithBaseURL(apiSrv.URL),
    zoho.WithTokenURL(tokenSrv.URL),
)
```

## Common mistakes

| Mistake | Correct behavior |
|---|---|
| Sending amounts in paise | Zoho Payments uses **rupees** (`4999.00`). Razorpay uses paise : easy cross-gateway bug. |
| `Authorization: Bearer ...` | Zoho uses `Zoho-oauthtoken <token>`. The SDK sets this automatically. |
| Scope `ZohoPayments.*` | Wrong product. Use `ZohoPay.*` (production) or `ZohoPaySandbox.*` (sandbox). |
| Missing `soid` in auth URL | Causes silent auth failure. Always include `soid=zohopay.<ACCOUNT_ID>`. |
| Using "Self Client" OAuth app | Fails : no redirect URI. Register a "Server-based Application". |
| Decoding `amount` as `float64` | Zoho returns `"4999.00"` as a string. Use the SDK's `Amount` type. |
| Decoding `created_time` as `time.Time` | Zoho returns epoch milliseconds. Use the SDK's `Time` type. |
| Expecting `reference_id` in `payment.succeeded` | Not present. Store `payment_link_id` at creation time and look up by it. |
| Handling only one webhook event | Both `payment_link.paid` and `payment.succeeded` fire per payment. Make writes idempotent. |
| Sharing sandbox and production credentials | Fully isolated environments : separate OAuth apps, tokens, signing keys, account IDs. |
| Confirming payment from a screenshot | Always confirm via webhook (verified signature) or `GetPaymentLink` / `GetPayment`. |

## License

MIT
