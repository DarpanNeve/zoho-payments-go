# zoho-payments-go

> **Unofficial SDK** — This is a community-maintained project and is not affiliated with, endorsed by, or supported by Zoho Corporation. "Zoho" and "Zoho Payments" are trademarks of Zoho Corporation Pvt. Ltd.

Production-grade Go SDK for [Zoho Payments](https://www.zoho.com/in/payments/) — payment links, payments, refunds, and webhooks. Built and verified against the live API (India data center), with the undocumented quirks already handled for you.

Working in Node.js/TypeScript instead? Use the sibling SDK [zoho-payments-node](../zoho-payments-node) — identical surface and semantics, fully interoperable (a link created by one is visible, cancelable and refundable by the other; webhooks verify identically).

- **Zero dependencies** — Go standard library only, Go 1.22+
- **OAuth handled internally** — refresh-token flow with in-memory caching, automatic re-auth on 401
- **Resilient by default** — exponential-backoff retries on 429/5xx for safe (GET) requests, context-aware throughout
- **Correct decoding** — Zoho returns money as decimal strings (`"4999.00"`) and timestamps as epoch milliseconds; the SDK's `Amount` and `Time` types absorb both so your structs just work
- **Webhook toolkit** — signature verification (with optional replay protection), typed event parsing
- **Fully tested** — `go test ./...` runs an offline test suite against a fake Zoho server

```
go get github.com/DarpanNeve/zoho-payments-go
```

```go
import zoho "github.com/DarpanNeve/zoho-payments-go"
```

---

## Table of contents

1. [Prerequisites: Zoho account setup](#1-prerequisites-zoho-account-setup)
2. [Getting OAuth credentials (one-time)](#2-getting-oauth-credentials-one-time)
3. [Creating the client](#3-creating-the-client)
4. [Payment links](#4-payment-links)
5. [Payments](#5-payments)
6. [Refunds](#6-refunds)
7. [Webhooks](#7-webhooks)
8. [Error handling](#8-error-handling)
9. [Retries and rate limits](#9-retries-and-rate-limits)
10. [Helpers](#10-helpers)
11. [Reconciliation (don't trust webhooks alone)](#11-reconciliation-dont-trust-webhooks-alone)
12. [Common pitfalls](#12-common-pitfalls)
13. [Environment variables](#13-environment-variables)
14. [Testing](#14-testing)

---

## 1. Prerequisites: Zoho account setup

You need four values before writing any code:

| Value | Where to find it |
|---|---|
| **Account ID** | Zoho Payments dashboard → Settings → Account Details |
| **Client ID** | Zoho API Console (step 2 below) |
| **Client Secret** | Zoho API Console (step 2 below) |
| **Refresh Token** | One-time browser flow (step 2 below) |

For webhooks you additionally need the **Signing Key** from Zoho Payments → Developer Space → Authentication Keys (shown once — store it immediately; it can be regenerated).

**Sandbox:** the sandbox environment is *not* enabled by default. Email `support@zohopayments.com` with your Account ID and ask for sandbox access (~1 business day). Sandbox and production are fully isolated — separate OAuth apps, refresh tokens, signing keys, and account IDs.

## 2. Getting OAuth credentials (one-time)

Zoho Payments uses OAuth 2.0 with a long-lived **refresh token**. You do this flow once per environment; the refresh token never expires (max 20 per account, oldest is evicted).

**Step 1 — Register a Server-based Application** at [api-console.zoho.in](https://api-console.zoho.in). Do **not** use a Self Client — that flow has no redirect URI and fails for Zoho Payments. Any redirect URI works for the one-time flow (e.g. `https://example.com/callback`). Note the Client ID and Client Secret.

**Step 2 — Authorize in a browser.** Open this URL (one line), logged in as the Zoho Payments account owner:

```
https://accounts.zoho.in/oauth/v2/auth
  ?response_type=code
  &client_id=YOUR_CLIENT_ID
  &scope=ZohoPay.payments.CREATE,ZohoPay.payments.READ,ZohoPay.payments.UPDATE
  &redirect_uri=YOUR_REDIRECT_URI
  &access_type=offline
  &soid=zohopay.YOUR_ACCOUNT_ID
```

Critical details:
- The scope prefix is **`ZohoPay.`** (sandbox: **`ZohoPaySandbox.`**). `ZohoPayments.*` is a different product and yields 401s.
- The **`soid` parameter is required**: `zohopay.<ACCOUNT_ID>` for production, `zohopaysandbox.<ACCOUNT_ID>` for sandbox.
- After you approve, the browser redirects to `YOUR_REDIRECT_URI?code=...`. That code **expires in 60 seconds** — exchange it immediately.

**Step 3 — Exchange the code for a refresh token:**

```bash
curl -s -X POST "https://accounts.zoho.in/oauth/v2/token" \
  -d "grant_type=authorization_code" \
  -d "client_id=YOUR_CLIENT_ID" \
  -d "client_secret=YOUR_CLIENT_SECRET" \
  -d "redirect_uri=YOUR_REDIRECT_URI" \
  -d "code=PASTE_CODE_HERE"
```

Save the `refresh_token` from the response. That's the last manual step — the SDK exchanges it for short-lived access tokens automatically from here on.

## 3. Creating the client

Create **one** client per process and share it (it is safe for concurrent use; it caches the access token internally). Do not construct a client per request.

```go
client, err := zoho.New(
    os.Getenv("ZOHO_ACCOUNT_ID"),
    os.Getenv("ZOHO_CLIENT_ID"),
    os.Getenv("ZOHO_CLIENT_SECRET"),
    os.Getenv("ZOHO_REFRESH_TOKEN"),
)
if err != nil {
    log.Fatal(err) // fails fast on any missing credential
}
```

Options:

```go
zoho.New(acc, id, secret, refresh,
    zoho.WithSandbox(),                              // paymentssandbox.zoho.in (India)
    zoho.WithRegion(zoho.RegionGlobal),              // payments.zoho.com + accounts.zoho.com
    zoho.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
    zoho.WithMaxRetries(3),                          // GET retries on 429/5xx (default 2)
    zoho.WithBaseURL("https://..."),                 // escape hatch / test servers
    zoho.WithTokenURL("https://..."),
)
```

Notes:
- Default region is India (`payments.zoho.in`, tokens via `accounts.zoho.in`). All India endpoints are `.in` — region mismatches are a classic source of `invalid_client` errors.
- `WithSandbox()` is documented for India only; combining it with `RegionGlobal` returns an error unless you supply `WithBaseURL` yourself.
- Every API call automatically carries `account_id` as a query parameter and the `Authorization: Zoho-oauthtoken <token>` header — you never touch tokens.

## 4. Payment links

### Create

```go
link, err := client.CreatePaymentLink(ctx, zoho.CreatePaymentLinkRequest{
    Amount:           4999.00,                       // RUPEES — not paise
    Currency:         "INR",                         // optional, defaults to INR
    Description:      zoho.SanitizeText("Kedarkantha Trek — 2 guests"),
    ReferenceID:      bookingID,                     // your DB id; comes back in webhooks
    Phone:            zoho.NormalizePhone("919876543210", "IN"), // → "9876543210"
    PhoneCountryCode: "IN",
    Email:            "customer@example.com",        // optional
    ExpiresAt:        "2026-07-15",                  // optional, yyyy-MM-dd, default +30 days
    ReturnURL:        "https://yoursite.com/thanks", // optional
    NotifyCustomer:   &zoho.NotifyCustomer{Email: false, SMS: false}, // suppress Zoho's own messages
    Configurations: &zoho.Configurations{            // optional: restrict methods
        AllowedPaymentMethods: []string{zoho.MethodUPI, zoho.MethodCard},
    },
})
if err != nil {
    // handle — nothing was charged
}

sendToCustomer(link.URL)        // hosted checkout page
saveForWebhooks(link.PaymentLinkID)
```

Client-side validation runs before any network call: amount must be > 0, description is required (≤ 500 chars), reference ID ≤ 100 chars. A link response missing `payment_link_id` or `url` is treated as an error — you will never store a half-created link.

**Phone gotcha:** WhatsApp-style numbers arrive as `919876543210` (12 digits). Zoho's checkout pre-fill only works with the bare 10-digit number plus `PhoneCountryCode: "IN"` — that's exactly what `zoho.NormalizePhone(p, "IN")` produces.

### Get / Update / Cancel

```go
link, err := client.GetPaymentLink(ctx, linkID)
if link.IsPaid() { ... }        // status: active | paid | canceled | expired

link, err = client.UpdatePaymentLink(ctx, linkID, zoho.UpdatePaymentLinkRequest{
    ExpiresAt: "2026-08-01",    // only active links can be updated
})

err = client.CancelPaymentLink(ctx, linkID) // PUT /paymentlinks/{id}/cancel
```

`CancelPaymentLink(ctx, "")` is a silent no-op — convenient when revoking "the other gateway's link" that may not exist.

### PaymentLink fields

`PaymentLinkID`, `URL`, `Amount`, `AmountPaid` (both `zoho.Amount`), `Currency`, `Status`, `Description`, `ReferenceID`, `Email`, `Phone`, `PhoneCountryCode`, `ExpiresAt` (string, `yyyy-MM-dd`), `ReturnURL`, `CreatedTime` (`zoho.Time`), `Payments []LinkPayment`.

## 5. Payments

```go
p, err := client.GetPayment(ctx, paymentID)
// p.Status: initiated | succeeded | failed | canceled | incomplete |
//           refunded | partially_refunded | blocked | disputed
fmt.Println(p.Amount.Float64(), p.FeeAmount.Float64(), p.NetAmount.Float64())
```

List with the documented filters (note the enum-style values — the SDK ships constants):

```go
payments, err := client.ListPayments(ctx, zoho.ListPaymentsParams{
    Status:   zoho.ListStatusSucceeded,   // "Status.Succeeded"
    FilterBy: zoho.FilterLast30Days,      // "ChargeDate.Last_30_Days"
    PerPage:  100,                        // capped at 200 by the API
    Page:     1,
})

// custom date range:
payments, err = client.ListPayments(ctx, zoho.ListPaymentsParams{
    FilterBy: zoho.FilterCustomDate,
    FromDate: "2026-06-01",               // required with CustomDate —
    ToDate:   "2026-06-11",               // validated client-side
})

// or free-text search by payment id / phone / email / reference:
payments, err = client.ListPayments(ctx, zoho.ListPaymentsParams{SearchText: "9876543210"})
```

## 6. Refunds

`Reason` and `Type` are **required by the API** — the SDK rejects the request locally if they're missing, so you find out in development, not production.

```go
refund, err := client.CreateRefund(ctx, paymentID, zoho.CreateRefundRequest{
    Amount:      1000.00,                              // partial refunds allowed
    Reason:      zoho.RefundReasonRequestedByCustomer, // duplicate | fraudulent |
                                                       // requested_by_customer | others | system_initiated
    Type:        zoho.RefundTypeMerchant,              // initiated_by_merchant | _customer | _system
    Description: "Trek cancelled due to weather",
})

refund, err = client.GetRefund(ctx, refund.RefundID)  // GET /refunds/{id}
// refund.Status: initiated | succeeded | failed | canceled | pending
// refund.FailureReason is populated when Status == failed
```

Refunds are asynchronous — `CreateRefund` typically returns `initiated`; track completion via the `refund.succeeded` / `refund.failed` webhooks or `GetRefund` polling.

## 7. Webhooks

Configure the endpoint in Zoho Payments → Developer Space → Webhooks, and copy the **Signing Key**.

### Complete handler

```go
func zohoWebhook(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    signingKey := os.Getenv("ZOHO_SIGNING_KEY")
    if signingKey == "" {
        http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
        return // never default-allow
    }

    r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }

    sig := r.Header.Get(zoho.SignatureHeader) // "X-Zoho-Webhook-Signature"
    if !zoho.VerifySignatureWithTolerance(body, sig, signingKey, 5*time.Minute) {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    ev, err := zoho.ParseEvent(body)
    if err != nil {
        w.WriteHeader(http.StatusOK) // malformed but authentic — ack so Zoho stops retrying
        return
    }

    // Ack BEFORE processing — Zoho retries on slow/non-200 responses.
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
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
        // pl.ReferenceID = the ReferenceID you set at creation (your booking id)
        markPaidByReference(ctx, pl.ReferenceID, pl.FirstPaymentID())

    case zoho.EventPaymentSucceeded:
        obj, err := ev.PaymentObject()
        if err != nil {
            return
        }
        // NOTE: this event has NO reference_id — look up by the stored link id.
        markPaidByLinkID(ctx, obj.Payment.PaymentLinkID, obj.Payment.PaymentID)

    case zoho.EventRefundSucceeded, zoho.EventRefundFailed:
        obj, _ := ev.RefundObject()
        updateRefundStatus(ctx, obj.Refund.RefundID, obj.Refund.Status)
    }
}
```

### Rules you must follow

1. **Verify before everything.** Signature scheme: header `X-Zoho-Webhook-Signature: t=<unix-ms>,v=<hex>`; signed string is `t + "." + rawBody`; HMAC-SHA256 with the Signing Key. `VerifySignature` does the constant-time comparison; the `WithTolerance` variant additionally rejects events older than your window (replay protection). Verify against the **raw** body — any re-serialization breaks the HMAC.
2. **Both `payment.succeeded` and `payment_link.paid` fire for every successful link payment** (in that order). Your "mark paid" write must be idempotent — e.g. a conditional update `{_id: ..., paymentStatus: {$ne: "PAID"}}` so the second event is a no-op and the customer gets one confirmation, not two.
3. **Ack fast.** Write 200 and flush before doing DB/network work, then process within a bounded context. Essential on serverless (Vercel/Lambda) where slow responses cause provider retries and duplicate processing.
4. **`ev.EventID` is your webhook-level dedup key** and `ev.LiveMode` distinguishes sandbox deliveries from production.

### Event constants

`EventPaymentSucceeded`, `EventPaymentFailed`, `EventPaymentLinkPaid`, `EventPaymentLinkExpired`, `EventPaymentLinkCanceled`, `EventRefundSucceeded`, `EventRefundFailed`, `EventVirtualAccountPaid`, `EventVirtualAccountClosed`, `EventPayoutInitiated`, `EventPayoutPaid`, `EventPayoutFailed`.

(There is no `refund.processed` event, regardless of what older integrations assume.)

## 8. Error handling

Every non-success API response becomes a `*zoho.APIError`:

```go
link, err := client.CreatePaymentLink(ctx, req)
if err != nil {
    var apiErr *zoho.APIError
    if errors.As(err, &apiErr) {
        switch {
        case apiErr.IsRateLimited(): // HTTP 429
            requeue()
        case apiErr.IsAuthError():   // 401/403 — check scopes / soid / region
            alertOps(apiErr)
        case apiErr.IsNotFound():    // 404
            handleMissing()
        default:
            log.Printf("zoho error code=%d/%s http=%d msg=%s",
                apiErr.Code, apiErr.CodeText, apiErr.HTTPStatus, apiErr.Message)
        }
        return
    }
    // non-API error: network failure, context cancellation, decode error
}
```

The error envelope handles Zoho's inconsistency where `code` is the integer `0` on success but sometimes the *string* `"error"` on failure — including failures delivered with HTTP 200.

## 9. Retries and rate limits

| Situation | Behavior |
|---|---|
| `401 Unauthorized` | Cached token invalidated, refreshed, request replayed **once** (any method — a 401 means the request never executed) |
| `429` / `5xx` / network error on **GET** | Retried with exponential backoff (400ms · 2ⁿ), up to `WithMaxRetries` (default 2), honoring context cancellation |
| `429` / `5xx` on **POST/PUT** | **Never auto-retried** — Zoho Payments has no idempotency keys, and retrying a create could double-charge or duplicate links. You decide. |

Zoho's documented limits: **600 requests/min** for payments, **60 requests/min** for refunds. Access tokens live ~1 hour; the SDK refreshes ~60s early under a mutex, so concurrent goroutines never stampede the token endpoint.

## 10. Helpers

```go
zoho.SanitizeText("Trek <Booking> & Stay!")   // "Trek Booking  Stay" — strips chars Zoho rejects
zoho.NormalizePhone("919876543210", "IN")     // "9876543210"
zoho.NormalizePhone("9876543210", "IN")       // unchanged
amount.Float64()                              // zoho.Amount → float64
link.CreatedTime.Time                         // zoho.Time embeds time.Time
```

Always run user-supplied text (names, trek titles) through `SanitizeText` before putting it in `Description` — Zoho rejects many special characters.

## 11. Reconciliation (don't trust webhooks alone)

Webhooks drop — networks fail, deploys race, signatures get rotated. For money flows, run a periodic sweep over pending orders:

```go
func reconcile(ctx context.Context, pending []Order) {
    for _, o := range pending {
        link, err := client.GetPaymentLink(ctx, o.ZohoLinkID)
        if err != nil {
            continue
        }
        if link.IsPaid() {
            markPaidByReference(ctx, link.ReferenceID, "") // same idempotent write as the webhook
        }
    }
}
```

Note: the API has **no list endpoint for payment links** — reconciliation iterates your own stored link IDs (as above) or uses `ListPayments` with a date filter.

## 12. Common pitfalls

| Pitfall | Reality |
|---|---|
| Sending paise | Zoho amounts are **rupees** (`4999.00`). Razorpay uses paise — easiest cross-gateway bug. |
| `Bearer` auth header | Zoho uses `Zoho-oauthtoken <token>` (SDK handles it). |
| Scope `ZohoPayments.*` | Wrong product → 401. Use `ZohoPay.*` / `ZohoPaySandbox.*`. |
| Missing `soid` in auth URL | Authorization silently misbinds. Always `soid=zohopay.<ACCOUNT_ID>`. |
| Self Client OAuth app | No redirect URI → flow fails. Register a **Server-based Application**. |
| Decoding `amount` as float / `created_time` as `time.Time` | Responses use decimal **strings** and **epoch ms**. Use the SDK structs as-is. |
| Expecting `reference_id` in `payment.succeeded` | It's not there. Store `PaymentLinkID` at creation and look up by it. |
| Handling only one webhook event | Both fire per payment. Idempotent writes or double confirmations. |
| Confirming payment from the customer's screenshot | Confirm only via verified webhook or `GetPaymentLink`/`GetPayment`. |
| Same credentials for sandbox + prod | Fully separate apps, tokens, signing keys, account IDs. |

## 13. Environment variables

Suggested names (matching this SDK's consumers):

```
ZOHO_ACCOUNT_ID      # dashboard → Settings → Account Details
ZOHO_CLIENT_ID       # api-console.zoho.in
ZOHO_CLIENT_SECRET
ZOHO_REFRESH_TOKEN   # from the one-time flow in §2
ZOHO_SIGNING_KEY     # Developer Space → Authentication Keys (webhooks)
ZOHO_SANDBOX=true    # your own flag → gate zoho.WithSandbox()
```

```go
opts := []zoho.Option{}
if os.Getenv("ZOHO_SANDBOX") == "true" {
    opts = append(opts, zoho.WithSandbox())
}
client, err := zoho.New(accountID, clientID, clientSecret, refreshToken, opts...)
```

## 14. Testing

```bash
go test ./...        # offline; fake token + API servers via httptest
go test -race ./...
```

Point the client at your own mock with the escape hatches:

```go
client, _ := zoho.New("acc", "id", "secret", "refresh",
    zoho.WithBaseURL(mockAPI.URL),
    zoho.WithTokenURL(mockToken.URL),
)
```

Design notes, doc-vs-production discrepancies, and migration guidance: [docs/LLM_CONTEXT.md](docs/LLM_CONTEXT.md).

## License

MIT
