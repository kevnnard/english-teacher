package domain

import "testing"

func TestAnswerMapsToSM2Grade(t *testing.T) {
	tests := []struct {
		answer Answer
		want   Grade
		name   string
	}{
		{Again, 1, "again"},
		{Hard, 3, "hard"},
		{Good, 4, "good"},
		{Easy, 5, "easy"},
	}
	for _, tt := range tests {
		if got := tt.answer.Grade(); got != tt.want {
			t.Errorf("%v.Grade() = %d, want %d", tt.answer, got, tt.want)
		}
		if got := tt.answer.String(); got != tt.name {
			t.Errorf("Answer(%d).String() = %q, want %q", int(tt.answer), got, tt.name)
		}
	}
	// Only "again" fails: hard is a pass that SM-2 schedules more cautiously.
	if Again.Grade().Passed() || !Hard.Grade().Passed() {
		t.Error("again must fail and hard must pass")
	}
}

func TestNewAnswer(t *testing.T) {
	for n := 1; n <= 4; n++ {
		a, err := NewAnswer(n)
		if err != nil || int(a) != n {
			t.Errorf("NewAnswer(%d) = %v, %v", n, a, err)
		}
	}
	for _, n := range []int{0, 5, -1} {
		if _, err := NewAnswer(n); err == nil {
			t.Errorf("NewAnswer(%d) should fail", n)
		}
	}
}
