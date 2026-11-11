package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestMistakeCountsStartEmpty(t *testing.T) {
	counts, err := openTemp(t).MistakeCounts(context.Background())
	if err != nil || len(counts) != 0 {
		t.Errorf("MistakeCounts = %v, %v; want none", counts, err)
	}
}

func TestRecordMistakesCountsEachKind(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	must(t, s.RecordMistakes(ctx, []domain.Mistake{
		{Date: aug14, Kind: "be", Wrong: "I has 30 years", Right: "I am 30 years old"},
		{Date: aug14, Kind: "article", Wrong: "I am teacher", Right: "I am a teacher"},
	}))
	must(t, s.RecordMistakes(ctx, []domain.Mistake{{Date: nov9, Kind: "be", Wrong: "she are", Right: "she is"}}))
	must(t, s.RecordMistakes(ctx, nil))

	counts, err := s.MistakeCounts(ctx)
	must(t, err)
	if want := map[string]int{"be": 2, "article": 1}; !reflect.DeepEqual(counts, want) {
		t.Errorf("MistakeCounts = %v, want %v", counts, want)
	}
	var date, wrong, right string
	must(t, s.db.QueryRow(`SELECT date, wrong, right FROM mistakes WHERE kind = 'be' ORDER BY id DESC LIMIT 1`).
		Scan(&date, &wrong, &right))
	if date != "2026-11-09" || wrong != "she are" || right != "she is" {
		t.Errorf("last be mistake = %s %q → %q", date, wrong, right)
	}
}

func TestMistakesSurviveReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	s, err := Open(path)
	must(t, err)
	must(t, s.RecordMistakes(context.Background(), []domain.Mistake{{Date: nov9, Kind: "plural", Wrong: "two dog", Right: "two dogs"}}))
	s.Close()

	s, err = Open(path)
	must(t, err)
	defer s.Close()
	counts, err := s.MistakeCounts(context.Background())
	if err != nil || counts["plural"] != 1 {
		t.Errorf("MistakeCounts = %v, %v", counts, err)
	}
}

// openV3 creates a database at schema version 3, before the mistakes log,
// with one card, one study day and a profile.
func openV3(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	must(t, err)
	defer db.Close()
	for _, q := range []string{
		migrations[0], migrations[1], migrations[2],
		`INSERT INTO cards (word, meaning) VALUES ('hello', 'Hola')`,
		`INSERT INTO reviews (card_id, direction, times_correct, due) VALUES (1, 'recognition', 3, '')`,
		`INSERT INTO days (date, minutes, reviews, activities) VALUES ('2026-11-09', 12, 4, 'Review')`,
		`INSERT INTO profile (id, name, level, started) VALUES (1, 'Kevin', 'A1', '2026-08-14')`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigration4KeepsAV3Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	openV3(t, path)

	s, err := Open(path)
	must(t, err)
	defer s.Close()
	ctx := context.Background()

	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version < 4 || version != len(migrations) {
		t.Errorf("user_version = %d, want len(migrations) %d", version, len(migrations))
	}
	cards, err := s.ListCards(ctx, domain.Recognition)
	must(t, err)
	if len(cards) != 1 || cards[0].Word != "hello" || cards[0].TimesCorrect != 3 {
		t.Errorf("cards = %+v", cards)
	}
	days, err := s.Days(ctx)
	must(t, err)
	if len(days) != 1 || days[0].Minutes != 12 || days[0].Reviews != 4 {
		t.Errorf("days = %+v", days)
	}
	p, err := s.Profile(ctx)
	must(t, err)
	if p.Name != "Kevin" || p.Started != aug14 {
		t.Errorf("profile = %+v", p)
	}
	must(t, s.RecordMistakes(ctx, []domain.Mistake{{Date: nov9, Kind: "be", Wrong: "I has", Right: "I have"}}))
	if counts, err := s.MistakeCounts(ctx); err != nil || counts["be"] != 1 {
		t.Errorf("MistakeCounts after migration = %v, %v", counts, err)
	}
}
