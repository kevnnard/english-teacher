package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestHomeGreetingFollowsTheClock(t *testing.T) {
	tests := []struct {
		hour int
		want string
	}{
		{hour: 8, want: "Good morning."},
		{hour: 14, want: "Good afternoon."},
		{hour: 21, want: "Good evening."},
	}
	for _, tt := range tests {
		clock := &testClock{t: time.Date(2026, 11, 9, tt.hour, 0, 0, 0, time.UTC)}
		a := NewApp(Config{Cards: deck(), Save: (&recorder{}).save, Now: clock.now})

		mustContain(t, screen(a), tt.want)
	}
}

func TestHomePreselectsReviewWithMinutes(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 120, 24)

	v := screen(a)

	mustContain(t, v, "▸ Review 5 words", "~3 min", "enter")
	if n := strings.Count(v, "▸"); n != 2 { // one in the rail, one action
		t.Errorf("want exactly one preselected action besides the rail cursor, found %d ▸:\n%s", n, v)
	}

	a, _ = press(t, a, "enter")
	if a.Screen() != Review {
		t.Errorf("enter went to %v, want Review", a.Screen())
	}
}

func TestHomeMinutesRoundUp(t *testing.T) {
	cards := deck()[:1] // one due word → ceil(0.5) = 1 min
	a := newApp(t, cards, &recorder{}, Home, 80, 24)

	mustContain(t, screen(a), "▸ Review 1 word", "~1 min")
}

func TestHomeWithNothingDuePreselectsWords(t *testing.T) {
	cards := []domain.Card{
		reviewed(word("goodbye", "", "Adiós", ""), day(1), 1),
		reviewed(word("thank you", "", "Gracias", ""), day(20), 30),
	}
	a := newApp(t, cards, &recorder{}, Home, 80, 24)

	v := screen(a)
	mustContain(t, v, "No reviews due — come back tomorrow", "▸ Browse 2 words")
	mustNotContain(t, v, "Review 0")
	if n := strings.Count(v, "▸"); n != 1 {
		t.Errorf("want exactly one preselected action, found %d:\n%s", n, v)
	}

	a, _ = press(t, a, "enter")
	if a.Screen() != Words {
		t.Errorf("enter went to %v, want Words", a.Screen())
	}
}

func TestHomeCountsWordsByStatus(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 120, 24)

	mustContain(t, screen(a), "● 3 due", "◐ 1 learning", "✓ 1 known", "○ 2 new")
}
