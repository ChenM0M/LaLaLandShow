package ticket

import (
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyQR(t *testing.T) {
	payload := newQRPayload("1121001", 42, 3, time.Unix(1000, 0).UTC())

	token, err := SignQR(payload, []byte("secret"))
	if err != nil {
		t.Fatalf("SignQR() error = %v", err)
	}

	got, err := VerifyQR(token, []byte("secret"))
	if err != nil {
		t.Fatalf("VerifyQR() error = %v", err)
	}
	if got != payload {
		t.Fatalf("payload = %#v, want %#v", got, payload)
	}
}

func TestVerifyQRRejectsTampering(t *testing.T) {
	payload := newQRPayload("1121001", 42, 3, time.Unix(1000, 0).UTC())
	token, err := SignQR(payload, []byte("secret"))
	if err != nil {
		t.Fatalf("SignQR() error = %v", err)
	}

	tampered := strings.TrimSuffix(token, token[len(token)-1:]) + "0"
	if _, err := VerifyQR(tampered, []byte("secret")); err != ErrInvalidQR {
		t.Fatalf("VerifyQR(tampered) error = %v, want %v", err, ErrInvalidQR)
	}
}
