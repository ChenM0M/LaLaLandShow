package student

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	students map[string]Student
}

func newFakeStore() *fakeStore {
	return &fakeStore{students: map[string]Student{}}
}

func (s *fakeStore) FindOrCreateStudent(_ context.Context, studentID string) (Student, error) {
	if student, ok := s.students[studentID]; ok {
		return student, nil
	}
	student := Student{StudentID: studentID, NeedRegister: true}
	s.students[studentID] = student
	return student, nil
}

func (s *fakeStore) UpdateStudentProfile(_ context.Context, studentID string, input RegisterInput) (Student, error) {
	student, ok := s.students[studentID]
	if !ok {
		return Student{}, ErrNotFound
	}
	student.ClassCode = &input.ClassCode
	student.SeatNoInClass = &input.SeatNoInClass
	student.DisplayName = input.DisplayName
	student.NeedRegister = false
	s.students[studentID] = student
	return student, nil
}

func (s *fakeStore) GetOverview(_ context.Context, studentID string) (Overview, error) {
	student, ok := s.students[studentID]
	if !ok {
		return Overview{}, ErrNotFound
	}
	return Overview{Student: student, ProxyBookings: []Booking{}}, nil
}

func (s *fakeStore) ClearPendingNotice(_ context.Context, studentID string) (Student, error) {
	student, ok := s.students[studentID]
	if !ok {
		return Student{}, ErrNotFound
	}
	student.PendingNotice = nil
	s.students[studentID] = student
	return student, nil
}

type fakeTokenIssuer struct{}

func (fakeTokenIssuer) GenerateStudentToken(studentID string, _ time.Time) (string, error) {
	return "token-for-" + studentID, nil
}

func TestServiceLoginCreatesSemiRegisteredStudent(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, fakeTokenIssuer{})

	result, err := service.Login(context.Background(), "1234567")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Token != "token-for-1234567" {
		t.Fatalf("Token = %q, want token-for-1234567", result.Token)
	}
	if !result.NeedRegister {
		t.Fatal("NeedRegister = false, want true for first login")
	}
}

func TestServiceSecondLoginKeepsCompletedProfile(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, fakeTokenIssuer{})

	if _, err := service.Login(context.Background(), "1234567"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	student, err := service.Register(context.Background(), "1234567", RegisterInput{
		ClassCode:     "1103",
		SeatNoInClass: 18,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if student.NeedRegister {
		t.Fatal("NeedRegister = true after Register(), want false")
	}

	result, err := service.Login(context.Background(), "1234567")
	if err != nil {
		t.Fatalf("Login() second time error = %v", err)
	}
	if result.NeedRegister {
		t.Fatal("NeedRegister = true on second login, want false")
	}
}

func TestServiceRejectsInvalidStudentID(t *testing.T) {
	service := NewService(newFakeStore(), fakeTokenIssuer{})

	if _, err := service.Login(context.Background(), "abc"); !errors.Is(err, ErrInvalidStudentID) {
		t.Fatalf("Login(invalid) error = %v, want %v", err, ErrInvalidStudentID)
	}
}

func TestValidateProfile(t *testing.T) {
	if err := ValidateProfile(RegisterInput{ClassCode: "1103", SeatNoInClass: 18}); err != nil {
		t.Fatalf("ValidateProfile(valid) error = %v", err)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "", SeatNoInClass: 18}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(empty class) error = %v, want %v", err, ErrInvalidProfile)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "1103"}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(empty seat no) error = %v, want %v", err, ErrInvalidProfile)
	}
}
