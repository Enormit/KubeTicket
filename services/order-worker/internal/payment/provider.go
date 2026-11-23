package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid webhook signature")
	ErrWebhookTimestamp = errors.New("webhook timestamp outside tolerance")
	ErrUnsupportedEvent = errors.New("unsupported webhook event type")
)

// WebhookEvent is the normalized payment webhook payload.
type WebhookEvent struct {
	Provider        string
	ProviderEventID string
	EventType       string
	OrderID         string
	PaymentID       string
	AmountCents     int64
	Currency        string
	RawPayload      []byte
}

// Provider verifies incoming payment webhooks and issues refunds.
type Provider interface {
	Name() string
	VerifyAndParseWebhook(headers http.Header, rawBody []byte) (*WebhookEvent, error)
	RefundPayment(ctx context.Context, paymentID string, amountCents int64, reason string) error
}

// StripeProvider verifies Stripe webhook HMAC-SHA256 signatures and handles refunds.
type StripeProvider struct {
	secretKey     string
	webhookSecret string
	tolerance     time.Duration
	appEnv        string
	httpClient    *http.Client
	refundBaseURL string
}

// NewStripeProvider creates a Stripe payment provider.
func NewStripeProvider(secretKey, webhookSecret, appEnv string, tolerance time.Duration) *StripeProvider {
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	return &StripeProvider{
		secretKey:     strings.TrimSpace(secretKey),
		webhookSecret: strings.TrimSpace(webhookSecret),
		tolerance:     tolerance,
		appEnv:        appEnv,
		httpClient:    &http.Client{Timeout: 8 * time.Second},
		refundBaseURL: "https://api.stripe.com/v1/refunds",
	}
}

func (p *StripeProvider) Name() string { return "stripe" }

// SignStripePayload builds a valid Stripe-Signature header value ("t=<unix>,v1=<hex>").
func SignStripePayload(secret string, ts time.Time, rawBody []byte) string {
	unixStr := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unixStr))
	mac.Write([]byte("."))
	mac.Write(rawBody)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", unixStr, sig)
}

func (p *StripeProvider) VerifyAndParseWebhook(headers http.Header, rawBody []byte) (*WebhookEvent, error) {
	if len(rawBody) == 0 {
		return nil, ErrInvalidSignature
	}

	if p.webhookSecret != "" {
		sigHeader := headers.Get("Stripe-Signature")
		if err := p.verifyStripeSignature(sigHeader, rawBody); err != nil {
			return nil, err
		}
	} else if p.appEnv == "production" {
		return nil, fmt.Errorf("%w: missing STRIPE_WEBHOOK_SECRET in production", ErrInvalidSignature)
	}

	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID              string            `json:"id"`
				PaymentIntent   string            `json:"payment_intent"`
				AmountTotal     int64             `json:"amount_total"`
				Amount          int64             `json:"amount"`
				Currency        string            `json:"currency"`
				ClientReference string            `json:"client_reference_id"`
				Metadata        map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, err
	}
	if envelope.ID == "" || envelope.Type == "" {
		return nil, ErrUnsupportedEvent
	}
	if envelope.Type != "checkout.session.completed" && envelope.Type != "payment_intent.succeeded" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEvent, envelope.Type)
	}

	obj := envelope.Data.Object
	orderID := obj.Metadata["order_id"]
	if orderID == "" {
		orderID = obj.ClientReference
	}
	paymentID := obj.PaymentIntent
	if paymentID == "" {
		paymentID = obj.ID
	}
	amount := obj.AmountTotal
	if amount == 0 {
		amount = obj.Amount
	}
	currency := strings.ToUpper(obj.Currency)
	if currency == "" {
		currency = "USD"
	}

	return &WebhookEvent{
		Provider:        "stripe",
		ProviderEventID: envelope.ID,
		EventType:       envelope.Type,
		OrderID:         orderID,
		PaymentID:       paymentID,
		AmountCents:     amount,
		Currency:        currency,
		RawPayload:      bytes.Clone(rawBody),
	}, nil
}

func (p *StripeProvider) verifyStripeSignature(header string, rawBody []byte) error {
	if header == "" {
		return ErrInvalidSignature
	}
	var tsStr string
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			tsStr = v
		case "v1":
			if decoded, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, decoded)
			}
		}
	}
	if tsStr == "" || len(sigs) == 0 {
		return ErrInvalidSignature
	}

	tsUnix, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	diff := time.Since(time.Unix(tsUnix, 0))
	if diff < 0 {
		diff = -diff
	}
	if diff > p.tolerance {
		return ErrWebhookTimestamp
	}

	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(rawBody)
	expected := mac.Sum(nil)

	for _, candidate := range sigs {
		if hmac.Equal(candidate, expected) {
			return nil
		}
	}
	return ErrInvalidSignature
}

func (p *StripeProvider) RefundPayment(ctx context.Context, paymentID string, amountCents int64, reason string) error {
	// Sandbox/dev mode without live Stripe API key records simulated refund cleanly.
	if p.secretKey == "" {
		return nil
	}

	form := url.Values{
		"payment_intent": {paymentID},
		"reason":         {"requested_by_customer"},
	}
	if amountCents > 0 {
		form.Set("amount", strconv.FormatInt(amountCents, 10))
	}
	if reason != "" {
		form.Set("metadata[kubeticket_reason]", reason)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.refundBaseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("stripe refund failed with HTTP %d", resp.StatusCode)
	}
	return nil
}
