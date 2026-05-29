package ticket

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidQR     = errors.New("invalid ticket qr")
	ErrInvalidSecret = errors.New("ticket hmac secret is empty")
)

type QRPayload struct {
	StudentID     string `json:"student_id"`
	SeatID        int64  `json:"seat_id"`
	TicketVersion int    `json:"ticket_version"`
	IssuedAt      string `json:"issued_at"`
}

func newQRPayload(studentID string, seatID int64, ticketVersion int, issuedAt time.Time) QRPayload {
	return QRPayload{
		StudentID:     studentID,
		SeatID:        seatID,
		TicketVersion: ticketVersion,
		IssuedAt:      issuedAt.UTC().Format(time.RFC3339),
	}
}

func SignQR(payload QRPayload, secret []byte) (string, error) {
	if len(secret) == 0 {
		return "", ErrInvalidSecret
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signature := sign(encodedPayload, secret)
	return encodedPayload + "." + hex.EncodeToString(signature), nil
}

func VerifyQR(token string, secret []byte) (QRPayload, error) {
	if len(secret) == 0 {
		return QRPayload{}, ErrInvalidSecret
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return QRPayload{}, ErrInvalidQR
	}
	got, err := hex.DecodeString(parts[1])
	if err != nil {
		return QRPayload{}, ErrInvalidQR
	}
	want := sign(parts[0], secret)
	if !hmac.Equal(got, want) {
		return QRPayload{}, ErrInvalidQR
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return QRPayload{}, ErrInvalidQR
	}
	var payload QRPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return QRPayload{}, ErrInvalidQR
	}
	if payload.StudentID == "" || payload.SeatID <= 0 || payload.TicketVersion <= 0 || payload.IssuedAt == "" {
		return QRPayload{}, ErrInvalidQR
	}
	return payload, nil
}

func sign(payload string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
