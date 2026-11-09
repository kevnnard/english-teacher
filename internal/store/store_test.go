package store

import (
	"context"
	"database/sql"
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

	got, err := s.ListCards(ctx, domain.Recognition)
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

	got, _ := s.ListCards(ctx, domain.Recognition)
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

	got, _ := s.ListCards(ctx, domain.Recognition)
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

	got, _ := s.ListCards(ctx, domain.Recognition)
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
	got, err := s.ListCards(ctx, domain.Recognition)
	if err != nil || len(got) != 1 {
		t.Errorf("after reopen got %d cards, err %v; want 1", len(got), err)
	}
}

// openV1 creates a database at the first schema version, as written by the
// app before directions existed, with one studied card.
func openV1(t *testing.T, path string, due, reviewed time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		migrations[0],
		`PRAGMA user_version = 1`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO cards (word, pronunciation, meaning, example, times_correct, times_failed,
		last_reviewed, ease_factor, interval_days, repetitions, due)
		VALUES ('hello', '/h/', 'Hola', 'Hello!', 4, 1, ?, 2.6, 6, 2, ?)`,
		reviewed.Format(timeLayout), due.Format(timeLayout))
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrationKeepsProgressAsRecognition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	due := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	reviewed := time.Date(2026, 11, 9, 21, 0, 0, 0, time.UTC)
	openV1(t, path, due, reviewed)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	rec, err := s.ListCards(ctx, domain.Recognition)
	if err != nil || len(rec) != 1 {
		t.Fatalf("recognition cards = %d, err %v; want 1", len(rec), err)
	}
	c := rec[0]
	if c.Word != "hello" || c.Meaning != "Hola" || c.Direction != domain.Recognition {
		t.Errorf("recognition card = %+v", c)
	}
	if c.TimesCorrect != 4 || c.TimesFailed != 1 || c.EaseFactor != 2.6 || c.IntervalDays != 6 ||
		c.Repetitions != 2 || !c.Due.Equal(due) || c.LastReviewed == nil || !c.LastReviewed.Equal(reviewed) {
		t.Errorf("progress lost in migration: %+v", c)
	}

	prod, err := s.ListCards(ctx, domain.Production)
	if err != nil || len(prod) != 1 {
		t.Fatalf("production cards = %d, err %v; want 1", len(prod), err)
	}
	p := prod[0]
	if p.Word != "hello" || p.Direction != domain.Production || p.LastReviewed != nil ||
		p.Repetitions != 0 || p.EaseFactor != domain.DefaultEaseFactor || !p.Due.IsZero() {
		t.Errorf("production should start fresh: %+v", p)
	}
}

func TestDirectionsHaveSeparateSchedules(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}

	reviewed := time.Date(2026, 11, 9, 21, 0, 0, 0, time.UTC)
	p := card("hello", "Hola")
	p.Direction = domain.Production
	p.TimesFailed, p.IntervalDays, p.EaseFactor = 1, 1, 2.18
	p.LastReviewed, p.Due = &reviewed, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	p.LastWrong = "helo"
	if err := s.SaveCard(ctx, p); err != nil {
		t.Fatalf("SaveCard production: %v", err)
	}

	prod, _ := s.ListCards(ctx, domain.Production)
	got := prod[0]
	if got.TimesFailed != 1 || got.EaseFactor != 2.18 || !got.Due.Equal(p.Due) || got.LastWrong != "helo" {
		t.Errorf("production card = %+v", got)
	}
	rec, _ := s.ListCards(ctx, domain.Recognition)
	if r := rec[0]; r.TimesFailed != 0 || r.LastReviewed != nil || r.LastWrong != "" || !r.Due.IsZero() {
		t.Errorf("saving production changed recognition: %+v", r)
	}

	// Saving again updates the same row instead of adding one.
	p.LastWrong = "hallo"
	if err := s.SaveCard(ctx, p); err != nil {
		t.Fatal(err)
	}
	prod, _ = s.ListCards(ctx, domain.Production)
	if len(prod) != 1 || prod[0].LastWrong != "hallo" {
		t.Errorf("after second save: %+v", prod)
	}
}

func TestReimportKeepsProductionSchedule(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}
	p := card("hello", "Hola")
	p.Direction, p.Repetitions, p.IntervalDays, p.LastWrong = domain.Production, 2, 6, "helo"
	if err := s.SaveCard(ctx, p); err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpsertCards(ctx, []domain.Card{card("HELLO", "Hola!")}); err != nil {
		t.Fatal(err)
	}

	prod, _ := s.ListCards(ctx, domain.Production)
	if c := prod[0]; c.Repetitions != 2 || c.IntervalDays != 6 || c.LastWrong != "helo" || c.Meaning != "Hola!" {
		t.Errorf("re-import reset production: %+v", c)
	}
}

func TestSaveCardUnknownWordInProduction(t *testing.T) {
	s := openTemp(t)
	c := card("ghost", "Fantasma")
	c.Direction = domain.Production

	if err := s.SaveCard(context.Background(), c); !errors.Is(err, ErrNotFound) {
		t.Errorf("SaveCard error = %v, want ErrNotFound", err)
	}
}
