package store

import (
	"context"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestAddCardsAddsOnlyNewWords(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	_, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")})
	must(t, err)

	added, err := s.AddCards(ctx, []domain.Card{card("Hello", "Otra cosa"), card("teacher", "profesor/a")})
	must(t, err)

	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}
	cards, _ := s.ListCards(ctx, domain.Recognition)
	if len(cards) != 2 || cards[0].Meaning != "Hola" || cards[1].Word != "teacher" || cards[1].LastReviewed != nil {
		t.Errorf("cards = %+v; an existing word must keep its text", cards)
	}
}

func TestAddCardsKeepsTheLastWrongInProduction(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	fix := card("I am 30 years old", "tengo 30 años")
	fix.LastWrong = "I has 30 years"

	_, err := s.AddCards(ctx, []domain.Card{fix})
	must(t, err)

	prod, _ := s.ListCards(ctx, domain.Production)
	rec, _ := s.ListCards(ctx, domain.Recognition)
	if len(prod) != 1 || prod[0].LastWrong != "I has 30 years" || prod[0].LastReviewed != nil {
		t.Errorf("production = %+v", prod)
	}
	if rec[0].LastWrong != "" {
		t.Errorf("recognition LastWrong = %q, want empty", rec[0].LastWrong)
	}
}

func TestAddCardsUpdatesTheLastWrongOfAnExistingWord(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	_, err := s.UpsertCards(ctx, []domain.Card{card("I'm from", "soy de")})
	must(t, err)
	prod := card("I'm from", "soy de")
	prod.Direction, prod.IntervalDays, prod.Repetitions = domain.Production, 6, 2
	must(t, s.SaveCard(ctx, prod))
	again := card("I'm from", "soy de")
	again.LastWrong = "I from"

	added, err := s.AddCards(ctx, []domain.Card{again})
	must(t, err)

	got, _ := s.ListCards(ctx, domain.Production)
	if added != 0 || got[0].LastWrong != "I from" || got[0].IntervalDays != 6 {
		t.Errorf("added %d, production = %+v; the schedule must stay", added, got[0])
	}
}

func TestAddCardsSkipsBlankWords(t *testing.T) {
	added, err := openTemp(t).AddCards(context.Background(), []domain.Card{card(" ", "x")})
	if err != nil || added != 0 {
		t.Errorf("added = %d, %v", added, err)
	}
}

func TestAddCardsStoresAChunkOnceInBothDirections(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	chunk := domain.Chunk{Phrase: "I'm looking for…", Meaning: "Estoy buscando…", Use: "en una tienda", Example: "I'm looking for a jacket."}

	first, err := s.AddCards(ctx, []domain.Card{chunk.Card()})
	must(t, err)
	again, err := s.AddCards(ctx, []domain.Card{chunk.Card()})
	must(t, err)

	if first != 1 || again != 0 {
		t.Errorf("added %d then %d, want 1 then 0", first, again)
	}
	for _, d := range []domain.Direction{domain.Recognition, domain.Production} {
		cards, err := s.ListCards(ctx, d)
		must(t, err)
		if len(cards) != 1 || cards[0].Word != "I'm looking for…" || cards[0].Meaning != "Estoy buscando… — en una tienda" ||
			cards[0].Example != "I'm looking for a jacket." || cards[0].LastReviewed != nil {
			t.Errorf("%v cards = %+v", d, cards)
		}
	}
}
