package domain

import (
	"testing"
	"time"
)

func TestDateOfUsesTheLocalCalendarDay(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	late := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC) // still the 9th in Bogotá

	if got := DateOf(late.In(bogota)); got != (Date{2026, time.November, 9}) {
		t.Errorf("DateOf = %v, want 2026-11-09", got)
	}
}

func TestDateStringAndParse(t *testing.T) {
	d, err := ParseDate("2026-08-14")
	if err != nil {
		t.Fatal(err)
	}
	if d != (Date{2026, time.August, 14}) || d.String() != "2026-08-14" {
		t.Errorf("ParseDate = %v (%s)", d, d)
	}
	if _, err := ParseDate("14/08/2026"); err == nil {
		t.Error("want an error for a date that is not YYYY-MM-DD")
	}
}

func TestDateAddDaysCrossesMonths(t *testing.T) {
	d := Date{2026, time.October, 31}

	if got := d.AddDays(1); got != (Date{2026, time.November, 1}) {
		t.Errorf("AddDays(1) = %v", got)
	}
	if got := d.AddDays(-31); got != (Date{2026, time.September, 30}) {
		t.Errorf("AddDays(-31) = %v", got)
	}
}

func TestDateBefore(t *testing.T) {
	a, b := Date{2026, time.August, 14}, Date{2026, time.November, 9}
	if !a.Before(b) || b.Before(a) || a.Before(a) {
		t.Error("Before should order dates by calendar day")
	}
}

func TestDayStudied(t *testing.T) {
	tests := []struct {
		name string
		day  Day
		want bool
	}{
		{"empty", Day{}, false},
		{"only minutes", Day{Minutes: 3}, false},
		{"a graded card", Day{Reviews: 1}, true},
		{"an activity", Day{Activities: []string{"Lesson 1"}}, true},
	}
	for _, tt := range tests {
		if got := tt.day.Studied(); got != tt.want {
			t.Errorf("%s: Studied = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	for _, s := range []string{"A1", "a2", " B1 ", "C2"} {
		if _, err := ParseLevel(s); err != nil {
			t.Errorf("ParseLevel(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "A3", "D1", "beginner"} {
		if _, err := ParseLevel(s); err == nil {
			t.Errorf("ParseLevel(%q) should fail", s)
		}
	}
	l, _ := ParseLevel("a1")
	if l != A1 || l.String() != "A1" || l.Name() != "Beginner" {
		t.Errorf("A1 = %q %q", l, l.Name())
	}
	if C2.Name() != "Proficiency" {
		t.Errorf("C2 name = %q", C2.Name())
	}
}

func TestNewProfileDefaults(t *testing.T) {
	p := NewProfile()
	if p.Level != A1 || p.DailyGoalMinutes != DefaultDailyGoalMinutes || p.Name != "" {
		t.Errorf("NewProfile = %+v", p)
	}
	if DefaultDailyGoalMinutes != 20 {
		t.Errorf("default goal = %d, want 20", DefaultDailyGoalMinutes)
	}
}
