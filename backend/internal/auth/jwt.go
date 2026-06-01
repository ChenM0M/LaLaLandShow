package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	studentIDLocalKey = "student_id"
	staffUserLocalKey = "staff_username"
	staffRoleLocalKey = "staff_role"
	defaultIssuer     = "lalalandshow"
)

var (
	ErrMissingToken = errors.New("missing bearer token")
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Manager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

type Claims struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	Issuer  string `json:"iss"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

func NewManager(secret string, ttl time.Duration) Manager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return Manager{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: defaultIssuer,
	}
}

func (m Manager) GenerateStudentToken(studentID string, now time.Time) (string, error) {
	return m.generateToken(studentID, "student", now)
}

func (m Manager) GenerateStaffToken(username, role string, now time.Time) (string, error) {
	if username == "" || (role != "checker" && role != "admin") {
		return "", ErrInvalidToken
	}
	return m.generateToken(username, role, now)
}

func (m Manager) generateToken(subject, role string, now time.Time) (string, error) {
	if len(m.secret) == 0 {
		return "", errors.New("jwt secret is empty")
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	claims := Claims{
		Subject: subject,
		Role:    role,
		Issuer:  m.issuer,
		Issued:  now.Unix(),
		Expires: now.Add(m.ttl).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	unsigned := encodeSegment(headerJSON) + "." + encodeSegment(claimsJSON)
	signature := signHS256([]byte(unsigned), m.secret)
	return unsigned + "." + encodeSegment(signature), nil
}

func (m Manager) ParseStudentToken(token string, now time.Time) (Claims, error) {
	claims, err := m.parse(token, now)
	if err != nil {
		return Claims{}, err
	}
	if claims.Role != "student" || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (m Manager) ParseStaffToken(token string, now time.Time, allowedRoles ...string) (Claims, error) {
	claims, err := m.parse(token, now)
	if err != nil {
		return Claims{}, err
	}
	if claims.Subject == "" || !isAllowedRole(claims.Role, allowedRoles) {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (m Manager) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		token, err := requestBearerToken(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}

		claims, err := m.ParseStudentToken(token, time.Now())
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}

		c.Locals(studentIDLocalKey, claims.Subject)
		return c.Next()
	}
}

func (m Manager) StaffMiddleware(allowedRoles ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		token, err := bearerToken(c.Get(fiber.HeaderAuthorization))
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}

		claims, err := m.ParseStaffToken(token, time.Now(), allowedRoles...)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}

		c.Locals(staffUserLocalKey, claims.Subject)
		c.Locals(staffRoleLocalKey, claims.Role)
		return c.Next()
	}
}

func StudentID(c fiber.Ctx) (string, bool) {
	value, ok := c.Locals(studentIDLocalKey).(string)
	return value, ok && value != ""
}

type StaffIdentity struct {
	Username string
	Role     string
}

func Staff(c fiber.Ctx) (StaffIdentity, bool) {
	username, ok := c.Locals(staffUserLocalKey).(string)
	if !ok || username == "" {
		return StaffIdentity{}, false
	}
	role, ok := c.Locals(staffRoleLocalKey).(string)
	if !ok || role == "" {
		return StaffIdentity{}, false
	}
	return StaffIdentity{Username: username, Role: role}, true
}

func bearerToken(header string) (string, error) {
	if header == "" {
		return "", ErrMissingToken
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", ErrMissingToken
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", ErrMissingToken
	}
	return token, nil
}

func requestBearerToken(c fiber.Ctx) (string, error) {
	token, err := bearerToken(c.Get(fiber.HeaderAuthorization))
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, ErrMissingToken) || c.Method() != fiber.MethodGet || c.Path() != "/api/seats/stream" {
		return "", err
	}
	queryToken := strings.TrimSpace(c.Query("token"))
	if queryToken == "" {
		return "", ErrMissingToken
	}
	return queryToken, nil
}

func (m Manager) parse(token string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	signature, err := decodeSegment(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	expected := signHS256([]byte(unsigned), m.secret)
	if !hmac.Equal(signature, expected) {
		return Claims{}, ErrInvalidToken
	}

	claimsJSON, err := decodeSegment(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Issuer != m.issuer {
		return Claims{}, ErrInvalidToken
	}
	if now.Unix() >= claims.Expires {
		return Claims{}, ErrExpiredToken
	}
	return claims, nil
}

func isAllowedRole(role string, allowedRoles []string) bool {
	if len(allowedRoles) == 0 {
		return role == "checker" || role == "admin"
	}
	for _, allowed := range allowedRoles {
		if role == allowed {
			return true
		}
	}
	return false
}

func signHS256(message, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(message)
	return mac.Sum(nil)
}

func encodeSegment(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeSegment(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}
