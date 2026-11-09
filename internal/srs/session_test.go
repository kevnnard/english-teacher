package srs

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var sessionNow = time.Date(2026, 11, 9, 21, 30, 0, 0, time.UTC)

func studied(word string, dir domain.Direction, reps int, due time.Time) domain.Card {
	c := domain.NewCard(word)
	last := sessionNow.AddDate(0, 0, -7)
	c.Direction, c.Repetitions, c.LastReviewed, c.Due = dir, reps, &last, due
	return c
}

func fresh(word string, dir domain.Direction) domain.Card {
	c := domain.NewCard(word)
	c.Direction = dir
	return c
}

func words(cards []domain.Card, idx []int) []string {
	var out []string
	for _, i := range idx {
		out = append(out, cards[i].Word+"/"+cards[i].Direction.String()[:4])
	}
	return out
}

func TestPlanSessionPutsDueCardsFirstAndCapsNewOnes(t *testing.T) {
	cards := []domain.Card{fresh("new1", domain.Recognition)}
	cards = append(cards, studied("due1", domain.Recognition, 1, sessionNow.Add(-time.Hour)))
	cards = append(cards, studied("later", domain.Recognition, 1, sessionNow.AddDate(0, 0, 2)))
	for i := 2; i <= 25; i++ {
		cards = append(cards, fresh(fmt.Sprintf("new%d", i), domain.Recognition))
	}
	cards = append(cards, studied("due2", domain.Recognition, 1, time.Time{}))

	got := words(cards, PlanSession(cards, sessionNow, 20))

	if len(got) != 22 {
		t.Fatalf("session has %d cards, want 2 due + 20 new: %v", len(got), got)
	}
	if !slices.Equal(got[:3], []string{"due1/reco", "due2/reco", "new1/reco"}) {
		t.Errorf("session starts %v, want due cards first, then new in order", got[:3])
	}
	if slices.Contains(got, "later/reco") || slices.Contains(got, "new21/reco") {
		t.Errorf("session %v should skip cards not due and new ones past the cap", got)
	}
}

func TestPlanSessionMaxNew(t *testing.T) {
	cards := []domain.Card{fresh("a", domain.Recognition), fresh("b", domain.Recognition), fresh("c", domain.Recognition)}

	for maxNew, want := range map[int]int{0: 0, 2: 2, 5: 3, -1: 0} {
		if got := len(PlanSession(cards, sessionNow, maxNew)); got != want {
			t.Errorf("maxNew %d: %d cards, want %d", maxNew, got, want)
		}
	}
}

func TestPlanSessionStartsProductionAfterTwoRecognitionRepetitions(t *testing.T) {
	future := sessionNow.AddDate(0, 0, 6)
	cards := []domain.Card{
		studied("hello", domain.Recognition, ProductionMinRepetitions, future),
		studied("hi", domain.Recognition, ProductionMinRepetitions-1, future),
		fresh("hello", domain.Production),
		fresh("hi", domain.Production),
		// Once started, a production card follows its own schedule.
		studied("goodbye", domain.Recognition, 5, future),
		studied("goodbye", domain.Production, 1, sessionNow.Add(-time.Minute)),
	}

	got := words(cards, PlanSession(cards, sessionNow, 20))

	if !slices.Equal(got, []string{"goodbye/prod", "hello/prod"}) {
		t.Errorf("session = %v, want due goodbye then new hello production only", got)
	}
}

func TestNextReview(t *testing.T) {
	tomorrow := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cards := []domain.Card{
		studied("a", domain.Recognition, 1, tomorrow),
		studied("b", domain.Production, 1, tomorrow.Add(5*time.Hour)),
		studied("c", domain.Recognition, 2, tomorrow.AddDate(0, 0, 5)),
		fresh("d", domain.Recognition), // never reviewed: not scheduled yet
	}

	day, n, ok := NextReview(cards, sessionNow)

	if !ok || !day.Equal(tomorrow) || n != 2 {
		t.Errorf("NextReview = %v, %d, %v; want tomorrow with 2 cards", day, n, ok)
	}
	if _, _, ok := NextReview([]domain.Card{fresh("d", domain.Recognition)}, sessionNow); ok {
		t.Error("NextReview with nothing scheduled should report false")
	}
}
