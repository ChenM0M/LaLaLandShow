package seat

import (
	"database/sql"
	"testing"
)

func TestClassifySnapshotSeat(t *testing.T) {
	tests := []struct {
		name              string
		isBlocked         bool
		bookingStudentID  sql.NullString
		bookedByStudentID sql.NullString
		customLabel       sql.NullString
		currentStudentID  string
		want              string
	}{
		{name: "free", want: StateFree},
		{name: "locked", isBlocked: true, want: StateLocked},
		{
			name:             "mine",
			bookingStudentID: sql.NullString{String: "1121001", Valid: true},
			currentStudentID: "1121001",
			want:             StateMine,
		},
		{
			name:             "others",
			bookingStudentID: sql.NullString{String: "1122001", Valid: true},
			currentStudentID: "1121001",
			want:             StateOthers,
		},
		{
			name:              "proxy",
			bookingStudentID:  sql.NullString{String: "1122001", Valid: true},
			bookedByStudentID: sql.NullString{String: "1121001", Valid: true},
			currentStudentID:  "1121001",
			want:              StateProxy,
		},
		{
			name:        "custom label occupied",
			customLabel: sql.NullString{String: "crew", Valid: true},
			want:        StateOthers,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySnapshotSeat(tt.isBlocked, tt.bookingStudentID, tt.bookedByStudentID, tt.customLabel, tt.currentStudentID)
			if got != tt.want {
				t.Fatalf("classifySnapshotSeat() = %q, want %q", got, tt.want)
			}
		})
	}
}
