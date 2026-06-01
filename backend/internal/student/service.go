package student

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidStudentID  = errors.New("student_id must be exactly seven digits")
	ErrInvalidProfile    = errors.New("class_code and seat_no_in_class are required")
	ErrIncompleteProfile = errors.New("student registration is incomplete")
	ErrNotFound          = errors.New("student not found")
)

var studentIDPattern = regexp.MustCompile(`^[0-9]{7}$`)

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
	student, err := s.store.ClearPendingNotice(ctx, studentID)
	if err != nil {
		return Student{}, fmt.Errorf("clear pending notice: %w", err)
	}
	return student, nil
}

func IsValidStudentID(studentID string) bool {
	return studentIDPattern.MatchString(studentID)
}

func ValidateProfile(input RegisterInput) error {
	if strings.TrimSpace(input.ClassCode) == "" || input.SeatNoInClass <= 0 {
		return ErrInvalidProfile
	}
	return nil
}

func HasCompleteProfile(student Student) bool {
	return student.ClassCode != nil &&
		strings.TrimSpace(*student.ClassCode) != "" &&
		student.SeatNoInClass != nil &&
		*student.SeatNoInClass > 0
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
