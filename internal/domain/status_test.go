package domain

import (
	"testing"
	"time"
)

func TestCardStatus(t *testing.T) {
	now := time.Date(2026, 11, 9, 21, 30, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -3)
	studied := func(interval int, due time.Time) Card {
		c := NewCard("hello")
		c.LastReviewed, c.IntervalDays, c.Due = &last, interval, due
		return c
	}

	tests := []struct {
		name string
		card Card
		want Status
	}{
		{name: "never reviewed", card: NewCard("hello"), want: StatusNew},
		{name: "due today", card: studied(1, now.Add(-time.Hour)), want: StatusDue},
		{name: "due without a date", card: studied(0, time.Time{}), want: StatusDue},
		{name: "scheduled soon", card: studied(6, now.AddDate(0, 0, 3)), want: StatusLearning},
		{name: "long interval", card: studied(KnownIntervalDays, now.AddDate(0, 0, 10)), want: StatusKnown},
		{name: "known but due again", card: studied(30, now.Add(-time.Minute)), want: StatusDue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.card.Status(now); got != tt.want {
				t.Errorf("Status = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	want := map[Status]string{StatusNew: "new", StatusDue: "due", StatusLearning: "learning", StatusKnown: "known"}
	for s, w := range want {
		if s.String() != w {
			t.Errorf("%d.String() = %q, want %q", s, s.String(), w)
		}
	}
}
