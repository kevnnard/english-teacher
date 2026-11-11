package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestPairStatsStartEmpty(t *testing.T) {
	stats, err := openTemp(t).PairStats(context.Background())
	if err != nil || len(stats) != 0 {
		t.Errorf("PairStats = %v, %v; want none", stats, err)
	}
}

func TestRecordPairResultsAddsToEachContrast(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	must(t, s.RecordPairResults(ctx, aug14, []domain.PairStat{
		{Contrast: "i-ee", Attempts: 5, Correct: 3},
		{Contrast: "b-v", Attempts: 4, Correct: 4},
	}))
	must(t, s.RecordPairResults(ctx, nov9, []domain.PairStat{{Contrast: "i-ee", Attempts: 2, Correct: 0}}))
	must(t, s.RecordPairResults(ctx, nov9, nil))

	stats, err := s.PairStats(ctx)
	must(t, err)
	want := map[string]domain.PairStat{
		"i-ee": {Contrast: "i-ee", Attempts: 7, Correct: 3},
		"b-v":  {Contrast: "b-v", Attempts: 4, Correct: 4},
	}
	if !reflect.DeepEqual(stats, want) {
		t.Errorf("PairStats = %v, want %v", stats, want)
	}
	var updated string
	must(t, s.db.QueryRow(`SELECT updated_at FROM pair_stats WHERE contrast = 'i-ee'`).Scan(&updated))
	if updated != "2026-11-09" {
		t.Errorf("updated_at = %q, want the last round's date", updated)
	}
}

// openV4 creates a database at schema version 4, before the pair stats,
// with a card, a study day, a profile and a logged mistake.
func openV4(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	must(t, err)
	defer db.Close()
	for _, q := range []string{
		migrations[0], migrations[1], migrations[2], migrations[3],
		`INSERT INTO cards (word, meaning) VALUES ('hello', 'Hola')`,
		`INSERT INTO reviews (card_id, direction, times_correct, due) VALUES (1, 'recognition', 3, '')`,
		`INSERT INTO days (date, minutes, reviews, activities) VALUES ('2026-11-09', 12, 4, 'Review')`,
		`INSERT INTO profile (id, name, level, started) VALUES (1, 'Kevin', 'A1', '2026-08-14')`,
		`INSERT INTO mistakes (date, kind, wrong, right) VALUES ('2026-11-09', 'be', 'I has', 'I have')`,
		`PRAGMA user_version = 4`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

// tableRows is every row of the tables migration 5 must not touch, as text.
func tableRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT id || word || pronunciation || meaning || example FROM cards`,
		`SELECT card_id || direction || times_correct || due || last_wrong FROM reviews`,
		`SELECT date || minutes || reviews || correct || failed || activities FROM days`,
		`SELECT id || name || level || started || daily_goal_minutes FROM profile`,
		`SELECT id || date || kind || wrong || right FROM mistakes`,
	} {
		rows, err := db.Query(q)
		must(t, err)
		for rows.Next() {
			var s string
			must(t, rows.Scan(&s))
			out = append(out, s)
		}
		must(t, rows.Err())
		rows.Close()
	}
	return out
}

func TestMigration5KeepsAV4Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	openV4(t, path)
	db, err := sql.Open("sqlite", path)
	must(t, err)
	before := tableRows(t, db)
	db.Close()

	s, err := Open(path)
	must(t, err)
	defer s.Close()
	ctx := context.Background()

	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version < 5 || version != len(migrations) {
		t.Errorf("user_version = %d, want every migration (%d), 5 included", version, len(migrations))
	}
	if after := tableRows(t, s.db); !reflect.DeepEqual(after, before) {
		t.Errorf("rows changed by migration 5:\n%q\nwant\n%q", after, before)
	}
	must(t, s.RecordPairResults(ctx, nov9, []domain.PairStat{{Contrast: "th-s", Attempts: 3, Correct: 1}}))
	if stats, err := s.PairStats(ctx); err != nil || stats["th-s"].Attempts != 3 {
		t.Errorf("PairStats after migration = %v, %v", stats, err)
	}
}

func TestMigration5OnAFreshDatabase(t *testing.T) {
	s := openTemp(t)
	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version < 5 || version != len(migrations) {
		t.Errorf("user_version = %d, want every migration (%d), 5 included", version, len(migrations))
	}
	var n int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM pair_stats`).Scan(&n))
	if n != 0 {
		t.Errorf("pair_stats has %d rows, want none", n)
	}
}

func TestPairStatsSurviveReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	s, err := Open(path)
	must(t, err)
	must(t, s.RecordPairResults(context.Background(), nov9, []domain.PairStat{{Contrast: "ed", Attempts: 2, Correct: 2}}))
	s.Close()

	s, err = Open(path)
	must(t, err)
	defer s.Close()
	stats, err := s.PairStats(context.Background())
	if err != nil || stats["ed"] != (domain.PairStat{Contrast: "ed", Attempts: 2, Correct: 2}) {
		t.Errorf("PairStats = %v, %v", stats, err)
	}
}
