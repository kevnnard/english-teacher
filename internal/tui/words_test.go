package tui

import (
	"strings"
	"testing"
)

func rowOf(t *testing.T, a App, w string) string {
	t.Helper()
	for _, l := range screenLines(a) {
		if strings.Contains(l, " "+w+" ") {
			return l
		}
	}
	t.Fatalf("no row for %q:\n%s", w, screen(a))
	return ""
}

func TestWordsTableShowsGlyphAndStatusWord(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Words, 120, 24)

	tests := map[string]string{
		"hello":     "● due",
		"Argentina": "○ new",
		"goodbye":   "◐ learning",
		"thank you": "✓ known",
	}
	for w, want := range tests {
		row := rowOf(t, a, w)
		if !strings.Contains(row, want) {
			t.Errorf("row for %q = %q, want %q", w, row, want)
		}
	}
	mustContain(t, rowOf(t, a, "hello"), "Hola")
}

func TestWordsFilter(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Words, 80, 24)

	a, _ = press(t, a, "/")
	a = typeText(t, a, "hol")

	v := screen(a)
	mustContain(t, v, "/ hol", "hello", "hi ")
	mustNotContain(t, v, "goodbye", "Argentina")

	// enter keeps the filter, esc on the filter clears it.
	a, _ = press(t, a, "enter")
	mustNotContain(t, screen(a), "goodbye")
	a, _ = press(t, a, "/", "esc")
	mustContain(t, screen(a), "goodbye")
	if a.Screen() != Words {
		t.Errorf("esc while filtering should stay on Words, got %v", a.Screen())
	}
}

func TestWordsFilterTypingDoesNotQuit(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Words, 80, 24)

	a, cmd := press(t, a, "/", "q")

	if isQuit(cmd) || a.Screen() != Words {
		t.Errorf("q while filtering should be text, got screen %v quit %v", a.Screen(), isQuit(cmd))
	}
	mustContain(t, screen(a), "/ q")
}

func TestWordsScrolls(t *testing.T) {
	a := newApp(t, longDeck(), &recorder{}, Words, 80, 8) // room for only a few rows

	if strings.Contains(screen(a), "thank you") {
		t.Fatalf("the last word should be off screen at height 8:\n%s", screen(a))
	}
	a, _ = press(t, a, "down", "down", "down", "down", "down", "down", "down", "down")
	mustContain(t, screen(a), "thank you")
}

func TestStatsCountsByStatus(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Stats, 80, 24)

	for _, want := range []string{"● due         3", "◐ learning    1", "✓ known       1", "○ new         2"} {
		mustContain(t, screen(a), want)
	}
}
