package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var (
	mon9  = domain.Date{Year: 2026, Month: time.November, Day: 9}
	tue10 = domain.Date{Year: 2026, Month: time.November, Day: 10}
)

func TestAddCardsStoresTheAddedDate(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	c := card("look for", "buscar")
	c.Added = mon9
	if _, err := s.AddCards(ctx, []domain.Card{c}); err != nil {
		t.Fatal(err)
	}
	// Adding the word again on another day keeps the first date.
	again := c
	again.Added = tue10
	if _, err := s.AddCards(ctx, []domain.Card{again}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []domain.Direction{domain.Recognition, domain.Production} {
		cards, err := s.ListCards(ctx, dir)
		must(t, err)
		if len(cards) != 1 || cards[0].Added != mon9 {
			t.Errorf("%v: cards = %+v, want one added on %v", dir, cards, mon9)
		}
	}
}

func TestImportedCardsHaveNoAddedDate(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if cards[0].Added != (domain.Date{}) {
		t.Errorf("Added = %v, want zero for an import", cards[0].Added)
	}
}

func TestSaveCardStoresKnownSince(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.UpsertCards(ctx, []domain.Card{card("hello", "Hola")}); err != nil {
		t.Fatal(err)
	}
	c := card("hello", "Hola")
	c.IntervalDays, c.KnownSince = 25, tue10
	must(t, s.SaveCard(ctx, c))
	cards, err := s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if cards[0].KnownSince != tue10 {
		t.Errorf("KnownSince = %v, want %v", cards[0].KnownSince, tue10)
	}
	c.IntervalDays, c.KnownSince = 1, domain.Date{}
	must(t, s.SaveCard(ctx, c))
	cards, err = s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if cards[0].KnownSince != (domain.Date{}) {
		t.Errorf("KnownSince = %v, want zero once forgotten", cards[0].KnownSince)
	}
}

func TestMistakesReturnsEveryDatedMistake(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	want := []domain.Mistake{
		{Date: mon9, Kind: "be", Wrong: "I has", Right: "I have"},
		{Date: tue10, Kind: "article", Wrong: "a apple", Right: "an apple"},
	}
	must(t, s.RecordMistakes(ctx, want))
	got, err := s.Mistakes(ctx)
	must(t, err)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mistakes = %+v, want %+v", got, want)
	}
}

func TestTickGoalKeepsTheFirstDate(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	ticks, err := s.GoalTicks(ctx)
	must(t, err)
	if len(ticks) != 0 {
		t.Errorf("GoalTicks = %v, want none", ticks)
	}
	must(t, s.TickGoal(ctx, "a1-read", mon9))
	must(t, s.TickGoal(ctx, "a1-read", tue10))
	ticks, err = s.GoalTicks(ctx)
	must(t, err)
	if !reflect.DeepEqual(ticks, map[string]domain.Date{"a1-read": mon9}) {
		t.Errorf("GoalTicks = %v, want a1-read on %v", ticks, mon9)
	}
}

// openV6 creates a database at schema version 6, before migration 7,
// with data in every table.
func openV6(t *testing.T, path string) {
	t.Helper()
	openV5(t, path)
	db, err := sql.Open("sqlite", path)
	must(t, err)
	defer db.Close()
	for _, q := range []string{
		migrations[5],
		`UPDATE reviews SET interval_days = 30`,
		`UPDATE days SET words_read = 320`,
		`INSERT INTO stories (title, chapter, text) VALUES ('The lost key', 2, 'Ana is a teacher.')`,
		`PRAGMA user_version = 6`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigration7KeepsAV6Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	openV6(t, path)
	db, err := sql.Open("sqlite", path)
	must(t, err)
	before := tableRows(t, db)
	var storyBefore string
	must(t, db.QueryRow(`SELECT title || chapter || text FROM stories`).Scan(&storyBefore))
	db.Close()

	s, err := Open(path)
	must(t, err)
	defer s.Close()
	ctx := context.Background()

	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version != 7 || version != len(migrations) {
		t.Errorf("user_version = %d, want 7 = len(migrations) %d", version, len(migrations))
	}
	if after := tableRows(t, s.db); !reflect.DeepEqual(after, before) {
		t.Errorf("rows changed by migration 7:\n%q\nwant\n%q", after, before)
	}
	var storyAfter string
	must(t, s.db.QueryRow(`SELECT title || chapter || text FROM stories`).Scan(&storyAfter))
	if storyAfter != storyBefore {
		t.Errorf("story = %q, want %q", storyAfter, storyBefore)
	}
	// Old cards have no dates: they were added and became known before
	// the dates were kept, so no week counts them.
	cards, err := s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if len(cards) != 1 || cards[0].IntervalDays != 30 || cards[0].Added != (domain.Date{}) || cards[0].KnownSince != (domain.Date{}) {
		t.Errorf("cards = %+v, want the v6 card with no dates", cards)
	}
	days, err := s.Days(ctx)
	must(t, err)
	if len(days) != 1 || days[0].WordsRead != 320 {
		t.Errorf("days = %+v, want the v6 day", days)
	}
	mistakes, err := s.Mistakes(ctx)
	must(t, err)
	if len(mistakes) != 1 || mistakes[0].Date != mon9 || mistakes[0].Kind != "be" {
		t.Errorf("mistakes = %+v, want the v6 mistake", mistakes)
	}
	must(t, s.TickGoal(ctx, "a1-read", mon9))
}

func TestMigration7OnAFreshDatabase(t *testing.T) {
	s := openTemp(t)
	var version, n int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version != 7 {
		t.Errorf("user_version = %d, want 7", version)
	}
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM goal_ticks`).Scan(&n))
	if n != 0 {
		t.Errorf("goal_ticks has %d rows, want none", n)
	}
	for table, column := range map[string]string{"cards": "added", "reviews": "known_since"} {
		must(t, s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n))
		if n != 1 {
			t.Errorf("%s has no %s column", table, column)
		}
	}
}

// The dates ride along with the existing fields: a save in one direction
// keeps every field of the card, and leaves the other direction alone.
func TestSaveCardAndAddCardsKeepTheirFieldsWithTheDates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	c := card("look for", "buscar")
	c.Added, c.LastWrong = mon9, "look"
	n, err := s.AddCards(ctx, []domain.Card{c})
	must(t, err)
	if n != 1 {
		t.Fatalf("added %d, want 1", n)
	}

	reviewed := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	prod := card("look for", "buscar")
	prod.Direction, prod.Added = domain.Production, mon9
	prod.EaseFactor, prod.IntervalDays, prod.Repetitions, prod.TimesCorrect = 2.6, 22, 4, 5
	prod.LastReviewed, prod.Due = &reviewed, reviewed.AddDate(0, 0, 22)
	prod.LastWrong, prod.KnownSince = "look", tue10
	must(t, s.SaveCard(ctx, prod))

	got, err := s.ListCards(ctx, domain.Production)
	must(t, err)
	g := got[0]
	if g.Word != "look for" || g.Meaning != "buscar" || g.Example != "look for!" || g.EaseFactor != 2.6 ||
		g.IntervalDays != 22 || g.Repetitions != 4 || g.TimesCorrect != 5 || g.LastWrong != "look" ||
		!g.Due.Equal(prod.Due) || g.LastReviewed == nil || !g.LastReviewed.Equal(reviewed) ||
		g.Added != mon9 || g.KnownSince != tue10 {
		t.Errorf("production = %+v", g)
	}
	rec, err := s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if r := rec[0]; r.IntervalDays != 0 || r.LastReviewed != nil || r.LastWrong != "" || r.KnownSince != (domain.Date{}) || r.Added != mon9 {
		t.Errorf("recognition = %+v, want the new card untouched", r)
	}
}
