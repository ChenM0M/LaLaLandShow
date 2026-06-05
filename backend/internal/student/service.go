package student

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidStudentID  = errors.New("student_id must match 110-115 + 1-4 + three digits")
	ErrInvalidProfile    = errors.New("class_code must be 701-705 through 1201-1205; seat_no_in_class must be 1-70")
	ErrIncompleteProfile = errors.New("student registration is incomplete")
	ErrNotFound          = errors.New("student not found")
	ErrAccountBanned     = errors.New("该账号学号信息真实性存疑，现已封禁、不予预约资格，如需申述请向1203相关负责人反馈")
)

var studentIDPattern = regexp.MustCompile(`^11[0-5][1-4][0-9]{3}$`)
var classCodePattern = regexp.MustCompile(`^(7|8|9|10|11|12)0[1-5]$`)

type Store interface {
	FindOrCreateStudent(ctx context.Context, studentID string) (Student, error)
	UpdateStudentProfile(ctx context.Context, studentID string, input RegisterInput) (Student, error)
	GetOverview(ctx context.Context, studentID string) (Overview, error)
	ClearPendingNotice(ctx context.Context, studentID string) (Student, error)
}

type TokenIssuer interface {
	GenerateStudentToken(studentID string, now time.Time) (string, error)
}

type Service struct {
	store  Store
	tokens TokenIssuer
	now    func() time.Time
}

func NewService(store Store, tokens TokenIssuer) *Service {
	return &Service{
		store:  store,
		tokens: tokens,
		now:    time.Now,
	}
}

func (s *Service) Login(ctx context.Context, studentID string) (LoginResult, error) {
	studentID = strings.TrimSpace(studentID)
	if !IsValidStudentID(studentID) {
		return LoginResult{}, ErrInvalidStudentID
	}

	student, err := s.store.FindOrCreateStudent(ctx, studentID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("find or create student: %w", err)
	}
	if IsAccountBanned(student.PendingNotice) {
		return LoginResult{}, ErrAccountBanned
	}
	token, err := s.tokens.GenerateStudentToken(student.StudentID, s.now())
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate token: %w", err)
	}

	return LoginResult{
		Token:         token,
		Student:       student,
		NeedRegister:  student.NeedRegister,
		PendingNotice: student.PendingNotice,
	}, nil
}

func (s *Service) Register(ctx context.Context, studentID string, input RegisterInput) (Student, error) {
	if !IsValidStudentID(studentID) {
		return Student{}, ErrInvalidStudentID
	}
	if err := ValidateProfile(input); err != nil {
		return Student{}, err
	}
	if err := s.ensureAccountActive(ctx, studentID); err != nil {
		return Student{}, err
	}

	student, err := s.store.UpdateStudentProfile(ctx, studentID, normalizeProfile(input))
	if err != nil {
		return Student{}, fmt.Errorf("update student profile: %w", err)
	}
	return student, nil
}

func (s *Service) Me(ctx context.Context, studentID string) (Overview, error) {
	if !IsValidStudentID(studentID) {
		return Overview{}, ErrInvalidStudentID
	}
	overview, err := s.store.GetOverview(ctx, studentID)
	if err != nil {
		return Overview{}, fmt.Errorf("get student overview: %w", err)
	}
	return overview, nil
}

func (s *Service) AcknowledgeNotice(ctx context.Context, studentID string) (Student, error) {
	if !IsValidStudentID(studentID) {
		return Student{}, ErrInvalidStudentID
	}
	if err := s.ensureAccountActive(ctx, studentID); err != nil {
		return Student{}, err
	}
	student, err := s.store.ClearPendingNotice(ctx, studentID)
	if err != nil {
		return Student{}, fmt.Errorf("clear pending notice: %w", err)
	}
	return student, nil
}

func IsValidStudentID(studentID string) bool {
	return studentIDPattern.MatchString(studentID)
}

func IsValidClassCode(classCode string) bool {
	return classCodePattern.MatchString(strings.TrimSpace(classCode))
}

func IsValidSeatNoInClass(seatNo int) bool {
	return seatNo >= 1 && seatNo <= 70
}

func ValidateProfile(input RegisterInput) error {
	if !IsValidClassCode(input.ClassCode) || !IsValidSeatNoInClass(input.SeatNoInClass) {
		return ErrInvalidProfile
	}
	return nil
}

func HasCompleteProfile(student Student) bool {
	return student.ClassCode != nil &&
		IsValidClassCode(*student.ClassCode) &&
		student.SeatNoInClass != nil &&
		IsValidSeatNoInClass(*student.SeatNoInClass)
}

func IsAccountBanned(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var notice struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &notice); err != nil {
		return false
	}
	return notice.Type == "admin_account_ban"
}

func (s *Service) ensureAccountActive(ctx context.Context, studentID string) error {
	overview, err := s.store.GetOverview(ctx, studentID)
	if err != nil {
		return fmt.Errorf("get student overview: %w", err)
	}
	if IsAccountBanned(overview.Student.PendingNotice) {
		return ErrAccountBanned
	}
	return nil
}

func normalizeProfile(input RegisterInput) RegisterInput {
	input.ClassCode = strings.TrimSpace(input.ClassCode)
	if input.DisplayName != nil {
		trimmed := strings.TrimSpace(*input.DisplayName)
		if trimmed == "" {
			input.DisplayName = nil
		} else {
			input.DisplayName = &trimmed
		}
	}
	return input
}
