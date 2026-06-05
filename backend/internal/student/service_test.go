package student

import (
	"context"
	"encoding/json"
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

	result, err := service.Login(context.Background(), "1121001")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Token != "token-for-1121001" {
		t.Fatalf("Token = %q, want token-for-1121001", result.Token)
	}
	if !result.NeedRegister {
		t.Fatal("NeedRegister = false, want true for first login")
	}
}

func TestServiceSecondLoginKeepsCompletedProfile(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, fakeTokenIssuer{})

	if _, err := service.Login(context.Background(), "1121001"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	student, err := service.Register(context.Background(), "1121001", RegisterInput{
		ClassCode:     "1104",
		SeatNoInClass: 18,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if student.NeedRegister {
		t.Fatal("NeedRegister = true after Register(), want false")
	}

	result, err := service.Login(context.Background(), "1121001")
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

func TestServiceLoginRejectsBannedAccount(t *testing.T) {
	store := newFakeStore()
	store.students["1121001"] = Student{
		StudentID:     "1121001",
		NeedRegister:  true,
		PendingNotice: json.RawMessage(`{"type":"admin_account_ban","reason":"suspicious"}`),
	}
	service := NewService(store, fakeTokenIssuer{})

	if _, err := service.Login(context.Background(), "1121001"); !errors.Is(err, ErrAccountBanned) {
		t.Fatalf("Login(banned) error = %v, want %v", err, ErrAccountBanned)
	}
}

func TestIsValidStudentID(t *testing.T) {
	tests := []struct {
		studentID string
		want      bool
	}{
		{studentID: "1101001", want: true},
		{studentID: "1123001", want: true},
		{studentID: "1154999", want: true},
		{studentID: "1091001", want: false},
		{studentID: "1161001", want: false},
		{studentID: "1125001", want: false},
		{studentID: "1121abc", want: false},
	}

	for _, tt := range tests {
		if got := IsValidStudentID(tt.studentID); got != tt.want {
			t.Fatalf("IsValidStudentID(%q) = %v, want %v", tt.studentID, got, tt.want)
		}
	}
}

func TestValidateProfile(t *testing.T) {
	if err := ValidateProfile(RegisterInput{ClassCode: "1104", SeatNoInClass: 18}); err != nil {
		t.Fatalf("ValidateProfile(valid) error = %v", err)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "", SeatNoInClass: 18}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(empty class) error = %v, want %v", err, ErrInvalidProfile)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "1104"}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(empty seat no) error = %v, want %v", err, ErrInvalidProfile)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "706", SeatNoInClass: 18}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(invalid class) error = %v, want %v", err, ErrInvalidProfile)
	}
	if err := ValidateProfile(RegisterInput{ClassCode: "1205", SeatNoInClass: 71}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("ValidateProfile(invalid seat no) error = %v, want %v", err, ErrInvalidProfile)
	}
	if !IsValidClassCode("701") || !IsValidClassCode("1205") {
		t.Fatal("IsValidClassCode rejected a valid boundary class")
	}
	if IsValidClassCode("706") || IsValidClassCode("1301") {
		t.Fatal("IsValidClassCode accepted an invalid class")
	}
}
