package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "teacher.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func card(word, meaning string) domain.Card {
	c := domain.NewCard(word)
	c.Pronunciation, c.Meaning, c.Example = "/"+word+"/", meaning, word+"!"
	return c
}

func TestUpsertAndList(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	reviewed := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	hello := card("hello", "Hola")
	hello.TimesCorrect, hello.LastReviewed = 1, &reviewed

	res, err := s.UpsertCards(ctx, []domain.Card{hello, card("hi", "Hola (informal)")})
	if err != nil {
		t.Fatalf("UpsertCards: %v", err)
	}
	if res.Added != 2 || res.Updated != 0 {
		t.Errorf("result = %+v, want 2 added, 0 updated", res)
	}

	got, err := s.ListCards(ctx)
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListCards returned %d cards, want 2", len(got))
	}
	h := got[0]
	if h.Word != "hello" || h.Meaning != "Hola" || h.Pronunciation != "/hello/" || h.Example != "hello!" {
		t.Errorf("first card = %+v, want hello", h)
	}
	if h.TimesCorrect != 1 || h.EaseFactor != domain.DefaultEaseFactor || !h.Due.IsZero() {
		t.Errorf("first card schedule = %+v, want imported defaults", h)
	}
	if h.LastReviewed == nil || !h.LastReviewed.Equal(reviewed) {
		t.Errorf("LastReviewed = %v, want %v", h.LastReviewed, reviewed)
	}
	if got[1].LastReviewed != nil {
		t.Errorf("hi LastReviewed = %v, want nil", got[1].LastReviewed)
	}
}

func TestUpsertIsCaseInsensitiveAndIdempotent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	batch := []domain.Card{card("Argentina", "País"), card("hello", "Hola")}

	if _, err := s.UpsertCards(ctx, batch); err != nil {
		t.Fatal(err)
	}
	res, err := s.UpsertCards(ctx, []domain.Card{card("ARGENTINA", "País: Argentina"), card("hello", "Hola")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || res.Updated != 2 {
		t.Errorf("re-import result = %+v, want 0 added, 2 updated", res)
	}

	got, _ := s.ListCards(ctx)
	if len(got) != 2 {
		t.Fatalf("after re-import got %d cards, want 2", len(got))
	}
	if got[0].Word != "Argentina" || got[0].Meaning != "País: Argentina" {
		t.Errorf("got %+v, want original word spelling with updated meaning", got[0])
	}
}

func TestReimportKeepsSchedule(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}

	reviewed := time.Date(2026, 11, 9, 21, 0, 0, 0, time.UTC)
	studied := card("hello", "Hola")
	studied.EaseFactor, studied.IntervalDays, studied.Repetitions = 2.6, 6, 2
	studied.TimesCorrect, studied.TimesFailed = 5, 1
	studied.LastReviewed = &reviewed
	studied.Due = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	if err := s.SaveCard(ctx, studied); err != nil {
		t.Fatalf("SaveCard: %v", err)
	}

	// The markdown bank still has the old counters and no review date.
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola!")}); err != nil {
		t.Fatal(err)
	}

	got, _ := s.ListCards(ctx)
	c := got[0]
	if c.Meaning != "Hola!" {
		t.Errorf("Meaning = %q, want content refreshed to %q", c.Meaning, "Hola!")
	}
	if c.EaseFactor != 2.6 || c.IntervalDays != 6 || c.Repetitions != 2 || !c.Due.Equal(studied.Due) {
		t.Errorf("schedule was reset: %+v", c)
	}
	if c.TimesCorrect != 5 || c.TimesFailed != 1 || c.LastReviewed == nil || !c.LastReviewed.Equal(reviewed) {
		t.Errorf("progress was reset: %+v", c)
	}
}

func TestSaveCardRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hi", "Hola")}); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("COT", -5*60*60)
	reviewed := time.Date(2026, 11, 9, 21, 30, 15, 0, loc)
	c := card("HI", "Hola")
	c.EaseFactor, c.IntervalDays, c.Repetitions, c.TimesFailed = 1.3, 1, 0, 3
	c.LastReviewed = &reviewed
	c.Due = time.Date(2026, 11, 10, 0, 0, 0, 0, loc)

	if err := s.SaveCard(ctx, c); err != nil {
		t.Fatalf("SaveCard: %v", err)
	}

	got, _ := s.ListCards(ctx)
	g := got[0]
	if g.Word != "hi" || g.EaseFactor != 1.3 || g.IntervalDays != 1 || g.TimesFailed != 3 {
		t.Errorf("got %+v", g)
	}
	if !g.Due.Equal(c.Due) || g.LastReviewed == nil || !g.LastReviewed.Equal(reviewed) {
		t.Errorf("times not preserved: due=%v last=%v", g.Due, g.LastReviewed)
	}
}

func TestSaveCardUnknownWord(t *testing.T) {
	s := openTemp(t)

	err := s.SaveCard(context.Background(), card("ghost", "Fantasma"))

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("SaveCard error = %v, want ErrNotFound", err)
	}
}

func TestOpenTwiceKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	ctx := context.Background()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path) // migrations must be safe to run again
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s.Close()
	got, err := s.ListCards(ctx)
	if err != nil || len(got) != 1 {
		t.Errorf("after reopen got %d cards, err %v; want 1", len(got), err)
	}
}
