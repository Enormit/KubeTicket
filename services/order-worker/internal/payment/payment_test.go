package payment

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStripeRawBodySignatureVerification(t *testing.T) {
	p := NewStripeProvider("", "whsec_test_secret", "production", 5*time.Minute)
	rawBody := []byte(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","payment_intent":"pi_1","amount_total":25000,"currency":"usd","metadata":{"order_id":"ord-123"}}}}`)

	now := time.Now()
	hdr := http.Header{}
	hdr.Set("Stripe-Signature", SignStripePayload("whsec_test_secret", now, rawBody))

	ev, err := p.VerifyAndParseWebhook(hdr, rawBody)
	if err != nil {
		t.Fatalf("expected valid webhook verification, got %v", err)
	}
	if ev.OrderID != "ord-123" || ev.PaymentID != "pi_1" || ev.AmountCents != 25000 {
		t.Fatalf("unexpected parsed webhook event: %+v", ev)
	}

	// Tampered body must fail HMAC check
	tampered := bytes.ReplaceAll(rawBody, []byte("25000"), []byte("100"))
	if _, err := p.VerifyAndParseWebhook(hdr, tampered); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature on tampered body, got %v", err)
	}

	// Expired timestamp beyond tolerance must fail
	oldHdr := http.Header{}
	oldHdr.Set("Stripe-Signature", SignStripePayload("whsec_test_secret", now.Add(-15*time.Minute), rawBody))
	if _, err := p.VerifyAndParseWebhook(oldHdr, rawBody); !errors.Is(err, ErrWebhookTimestamp) {
		t.Fatalf("expected ErrWebhookTimestamp, got %v", err)
	}
}

func TestWebhookHTTPHandlerRejectsBadSignature(t *testing.T) {
	p := NewStripeProvider("", "whsec_test_secret", "production", 5*time.Minute)
	h := NewHandler(nil, p)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payment", bytes.NewBufferString(`{"id":"evt_1"}`))
	req.Header.Set("Stripe-Signature", "t=123,v1=deadbeef")
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized for bad signature, got %d", rec.Code)
	}
}

func TestStripeProviderRefundHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_1"}`))
	}))
	defer srv.Close()

	p := NewStripeProvider("sk_test_123", "whsec", "development", 5*time.Minute)
	p.refundBaseURL = srv.URL

	if err := p.RefundPayment(context.Background(), "pi_1", 10000, "order_expired"); err != nil {
		t.Fatalf("unexpected refund error: %v", err)
	}
	if !called {
		t.Fatalf("expected Stripe refund endpoint to be called")
	}
}
