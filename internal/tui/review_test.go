package tui

import (
	"errors"
	"testing"
)

func TestReviewFrontShowsWordAndPronunciationOnly(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	v := screen(a)

	mustContain(t, v, "hello", "/həˈloʊ/", "1 / 5")
	mustNotContain(t, v, "Hola", "Hello, how are you?", "good")
}

func TestReviewSpaceRevealsMeaningAndExample(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	a, _ = press(t, a, "space")

	mustContain(t, screen(a), "hello", "Hola", "Hello, how are you?")
}

func TestReviewGradeBeforeRevealIsIgnored(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "4")

	if len(rec.saved) != 0 {
		t.Errorf("saved %d cards before reveal, want 0", len(rec.saved))
	}
	mustContain(t, screen(a), "1 / 5")
}

func TestReviewGradeSavesAndAdvances(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "space", "4")

	if len(rec.saved) != 1 {
		t.Fatalf("saved %d cards, want 1", len(rec.saved))
	}
	c := rec.saved[0]
	if c.Word != "hello" || c.TimesCorrect != 2 || c.LastReviewed == nil || !c.LastReviewed.Equal(fixedNow) {
		t.Errorf("saved %+v", c)
	}
	v := screen(a)
	mustContain(t, v, "hi", "2 / 5")
	mustNotContain(t, v, "Hola (informal)")
}

func TestReviewProgressSurvivesLeavingTheScreen(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	a, _ = press(t, a, "space", "4", "esc", "enter")

	if a.Screen() != Review {
		t.Fatalf("enter on Home should resume the review, got %v", a.Screen())
	}
	mustContain(t, screen(a), "2 / 5")
	a, _ = press(t, a, "esc")
	mustContain(t, screen(a), "Review 4 words")
}

func TestReviewWithNothingDue(t *testing.T) {
	a := newApp(t, deck()[5:], &recorder{}, Review, 80, 24)

	mustContain(t, screen(a), "No reviews due — come back tomorrow")
}

func TestReviewSaveErrorIsShownAndStopsTheApp(t *testing.T) {
	rec := &recorder{err: errors.New("disk full")}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "space", "4")

	mustContain(t, screen(a), "disk full")
	if a.Err() == nil {
		t.Error("Err() = nil, want the save error")
	}
	if _, cmd := press(t, a, "space"); !isQuit(cmd) {
		t.Error("any key after a save error should quit")
	}
}
