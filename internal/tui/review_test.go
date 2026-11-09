package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var fixedNow = time.Date(2026, 11, 9, 21, 30, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// key builds a synthetic key press, like the terminal would send.
func key(s string) tea.KeyPressMsg {
	if s == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// recorder is a fake save function that remembers every saved card.
type recorder struct {
	saved []domain.Card
	err   error
}

func (r *recorder) save(c domain.Card) error {
	r.saved = append(r.saved, c)
	return r.err
}

func testCards() []domain.Card {
	hello := domain.NewCard("hello")
	hello.Pronunciation, hello.Meaning, hello.Example = "/həˈloʊ/", "Hola", "Hello, how are you?"
	hi := domain.NewCard("hi")
	hi.Pronunciation, hi.Meaning, hi.Example = "/haɪ/", "Hola (informal)", "Hi, nice to meet you!"
	return []domain.Card{hello, hi}
}

// press feeds keys to m in order and returns the final model and last command.
func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(key(k))
		m = next.(Model)
	}
	return m, cmd
}

func content(m Model) string { return m.View().Content }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestKeyStrings(t *testing.T) {
	// Guards our synthetic keys against the strings Update matches on.
	if got := key("space").String(); got != "space" {
		t.Errorf("space key String() = %q", got)
	}
	if got := key("q").String(); got != "q" {
		t.Errorf("q key String() = %q", got)
	}
}

func TestFrontShowsWordAndPronunciationOnly(t *testing.T) {
	rec := &recorder{}
	m := New(testCards(), rec.save, clock)

	v := content(m)

	for _, want := range []string{"hello", "/həˈloʊ/", "1/2"} {
		if !strings.Contains(v, want) {
			t.Errorf("front view missing %q:\n%s", want, v)
		}
	}
	for _, hidden := range []string{"Hola", "Hello, how are you?", "4 good"} {
		if strings.Contains(v, hidden) {
			t.Errorf("front view should not show %q yet:\n%s", hidden, v)
		}
	}
}

func TestSpaceRevealsMeaningExampleAndHint(t *testing.T) {
	m, _ := press(t, New(testCards(), (&recorder{}).save, clock), "space")

	v := content(m)

	for _, want := range []string{"hello", "Hola", "Hello, how are you?", "0-2 forgot · 3 hard · 4 good · 5 easy"} {
		if !strings.Contains(v, want) {
			t.Errorf("revealed view missing %q:\n%s", want, v)
		}
	}
}

func TestGradeBeforeRevealIsIgnored(t *testing.T) {
	rec := &recorder{}

	m, _ := press(t, New(testCards(), rec.save, clock), "5")

	if len(rec.saved) != 0 {
		t.Errorf("saved %d cards before reveal, want 0", len(rec.saved))
	}
	if !strings.Contains(content(m), "1/2") {
		t.Errorf("should still be on the first card:\n%s", content(m))
	}
}

func TestGradeSavesRescheduledCardAndAdvances(t *testing.T) {
	tests := []struct {
		grade        string
		wantInterval int
		wantCorrect  int
		wantFailed   int
	}{
		{grade: "0", wantInterval: 1, wantFailed: 1},
		{grade: "2", wantInterval: 1, wantFailed: 1},
		{grade: "3", wantInterval: 1, wantCorrect: 1},
		{grade: "4", wantInterval: 1, wantCorrect: 1},
		{grade: "5", wantInterval: 1, wantCorrect: 1},
	}
	for _, tt := range tests {
		t.Run("grade "+tt.grade, func(t *testing.T) {
			rec := &recorder{}

			m, _ := press(t, New(testCards(), rec.save, clock), "space", tt.grade)

			if len(rec.saved) != 1 {
				t.Fatalf("saved %d cards, want 1", len(rec.saved))
			}
			c := rec.saved[0]
			if c.Word != "hello" || c.IntervalDays != tt.wantInterval ||
				c.TimesCorrect != tt.wantCorrect || c.TimesFailed != tt.wantFailed {
				t.Errorf("saved %+v", c)
			}
			if c.LastReviewed == nil || !c.LastReviewed.Equal(fixedNow) {
				t.Errorf("LastReviewed = %v, want injected clock %v", c.LastReviewed, fixedNow)
			}
			v := content(m)
			if !strings.Contains(v, "hi") || !strings.Contains(v, "2/2") || strings.Contains(v, "Hola (informal)") {
				t.Errorf("want the front of the second card:\n%s", v)
			}
		})
	}
}

func TestFinishingAllCardsShowsEndScreen(t *testing.T) {
	rec := &recorder{}

	m, cmd := press(t, New(testCards(), rec.save, clock), "space", "4", "space", "1")

	if len(rec.saved) != 2 {
		t.Errorf("saved %d cards, want 2", len(rec.saved))
	}
	if !strings.Contains(content(m), "No cards due. See you tomorrow!") {
		t.Errorf("end screen missing:\n%s", content(m))
	}
	if isQuit(cmd) {
		t.Error("should stay on the end screen until a key is pressed")
	}
	if _, cmd = press(t, m, "x"); !isQuit(cmd) {
		t.Error("any key on the end screen should quit")
	}
}

func TestNoCardsDue(t *testing.T) {
	m := New(nil, (&recorder{}).save, clock)

	if !strings.Contains(content(m), "No cards due. See you tomorrow!") {
		t.Errorf("empty session view:\n%s", content(m))
	}
}

func TestQuit(t *testing.T) {
	for _, keys := range [][]string{{"q"}, {"space", "q"}} {
		rec := &recorder{}

		_, cmd := press(t, New(testCards(), rec.save, clock), keys...)

		if !isQuit(cmd) {
			t.Errorf("keys %v: want quit", keys)
		}
		if len(rec.saved) != 0 {
			t.Errorf("keys %v: quitting saved %d cards, want 0", keys, len(rec.saved))
		}
	}
}

func TestCtrlCQuits(t *testing.T) {
	m := New(testCards(), (&recorder{}).save, clock)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if !isQuit(cmd) {
		t.Error("ctrl+c should quit")
	}
}

func TestSaveErrorIsShownAndStopsReview(t *testing.T) {
	rec := &recorder{err: errors.New("disk full")}

	m, _ := press(t, New(testCards(), rec.save, clock), "space", "4")

	if !strings.Contains(content(m), "disk full") {
		t.Errorf("error not shown:\n%s", content(m))
	}
	if m.Err() == nil {
		t.Error("Err() = nil, want the save error")
	}
	if _, cmd := press(t, m, "space"); !isQuit(cmd) {
		t.Error("any key after an error should quit")
	}
}
