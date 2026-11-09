package domain

import "testing"

func TestNewCardDefaults(t *testing.T) {
	c := NewCard("hello")

	if c.Word != "hello" {
		t.Errorf("Word = %q, want %q", c.Word, "hello")
	}
	if c.EaseFactor != DefaultEaseFactor {
		t.Errorf("EaseFactor = %v, want %v", c.EaseFactor, DefaultEaseFactor)
	}
	if c.IntervalDays != 0 || c.Repetitions != 0 {
		t.Errorf("new card should have no schedule, got interval=%d reps=%d", c.IntervalDays, c.Repetitions)
	}
	if c.LastReviewed != nil {
		t.Errorf("LastReviewed = %v, want nil", c.LastReviewed)
	}
	if !c.Due.IsZero() {
		t.Errorf("Due = %v, want zero time (due immediately)", c.Due)
	}
}

func TestNewGrade(t *testing.T) {
	tests := []struct {
		in      int
		want    Grade
		wantErr bool
	}{
		{in: 0, want: 0},
		{in: 3, want: 3},
		{in: 5, want: 5},
		{in: -1, wantErr: true},
		{in: 6, wantErr: true},
	}
	for _, tt := range tests {
		got, err := NewGrade(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewGrade(%d) error = nil, want error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("NewGrade(%d) unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("NewGrade(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestGradeValid(t *testing.T) {
	tests := []struct {
		g    Grade
		want bool
	}{
		{-1, false}, {0, true}, {2, true}, {5, true}, {6, false},
	}
	for _, tt := range tests {
		if got := tt.g.Valid(); got != tt.want {
			t.Errorf("Grade(%d).Valid() = %v, want %v", tt.g, got, tt.want)
		}
	}
}

func TestGradePassed(t *testing.T) {
	tests := []struct {
		g    Grade
		want bool
	}{
		{0, false}, {2, false}, {3, true}, {5, true},
	}
	for _, tt := range tests {
		if got := tt.g.Passed(); got != tt.want {
			t.Errorf("Grade(%d).Passed() = %v, want %v", tt.g, got, tt.want)
		}
	}
}
