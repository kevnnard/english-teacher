package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func sampleStory() domain.Story {
	return domain.Story{
		Title:   "The lost key",
		Chapter: 1,
		Summary: "Ana loses her key on Monday.",
		Text:    "Ana is a teacher. On Monday she cannot find her key.",
		Details: `{"glossary":[],"question":{"question":"What does Ana lose?"}}`,
	}
}

func TestCurrentStoryStartsEmpty(t *testing.T) {
	s := openTemp(t)
	if _, ok, err := s.CurrentStory(context.Background()); err != nil || ok {
		t.Errorf("CurrentStory = ok %v, err %v; want no story", ok, err)
	}
}

func TestSaveStoryInsertsThenUpdates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	st := sampleStory()
	id, err := s.SaveStory(ctx, st)
	must(t, err)
	if id == 0 {
		t.Fatal("SaveStory returned id 0 for a new story")
	}
	st.ID = id
	got, ok, err := s.CurrentStory(ctx)
	must(t, err)
	if !ok || !reflect.DeepEqual(got, st) {
		t.Fatalf("CurrentStory = %+v, %v; want %+v", got, ok, st)
	}

	// The chapter is answered, then chapter 2 replaces it in the same row.
	st.Done = true
	if id2, err := s.SaveStory(ctx, st); err != nil || id2 != id {
		t.Fatalf("update: id %d, err %v; want id %d", id2, err, id)
	}
	st.Chapter, st.Text, st.Summary, st.Done = 2, "Her cat has the key!", "Ana loses her key. Her cat has it.", false
	_, err = s.SaveStory(ctx, st)
	must(t, err)
	got, _, err = s.CurrentStory(ctx)
	must(t, err)
	if !reflect.DeepEqual(got, st) {
		t.Errorf("after chapter 2 = %+v, want %+v", got, st)
	}
	var n int
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&n))
	if n != 1 {
		t.Errorf("stories has %d rows, want 1", n)
	}
}

func TestCurrentStoryIsTheLatest(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	old := sampleStory()
	_, err := s.SaveStory(ctx, old)
	must(t, err)
	next := sampleStory()
	next.Title = "A day at the sea"
	id, err := s.SaveStory(ctx, next)
	must(t, err)
	got, _, err := s.CurrentStory(ctx)
	must(t, err)
	if got.ID != id || got.Title != "A day at the sea" {
		t.Errorf("CurrentStory = %+v, want the newest story", got)
	}
}

func TestStorySurvivesReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	ctx := context.Background()
	s, err := Open(path)
	must(t, err)
	st := sampleStory()
	st.ID, err = s.SaveStory(ctx, st)
	must(t, err)
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, WordsRead: 312, Activities: []string{"Reading: The lost key ch 1"}}))
	must(t, s.Close())

	s, err = Open(path)
	must(t, err)
	defer s.Close()
	got, ok, err := s.CurrentStory(ctx)
	must(t, err)
	if !ok || !reflect.DeepEqual(got, st) {
		t.Errorf("after reopening = %+v, want %+v", got, st)
	}
	days, err := s.Days(ctx)
	must(t, err)
	if len(days) != 1 || days[0].WordsRead != 312 {
		t.Errorf("days = %+v, want 312 words read", days)
	}
}

func TestRecordStudySumsWordsRead(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, WordsRead: 300}))
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, WordsRead: 250, Minutes: 4}))
	days, err := s.Days(ctx)
	must(t, err)
	if len(days) != 1 || days[0].WordsRead != 550 || days[0].Minutes != 4 {
		t.Errorf("days = %+v, want 550 words read", days)
	}
}

func TestMergeDaysKeepsTheLargerWordsRead(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, WordsRead: 300}))
	_, err := s.MergeDays(ctx, []domain.Day{{Date: nov9, WordsRead: 100}})
	must(t, err)
	days, err := s.Days(ctx)
	must(t, err)
	if days[0].WordsRead != 300 {
		t.Errorf("WordsRead = %d, want 300 (merging never adds up)", days[0].WordsRead)
	}
}

// openV5 creates a database at schema version 5, before stories, with
// data in every table.
func openV5(t *testing.T, path string) {
	t.Helper()
	openV4(t, path)
	db, err := sql.Open("sqlite", path)
	must(t, err)
	defer db.Close()
	for _, q := range []string{
		migrations[4],
		`INSERT INTO pair_stats (contrast, attempts, correct, updated_at) VALUES ('i-ee', 5, 4, '2026-11-09')`,
		`PRAGMA user_version = 5`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigration6KeepsAV5Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	openV5(t, path)
	db, err := sql.Open("sqlite", path)
	must(t, err)
	before := tableRows(t, db)
	var pairsBefore string
	must(t, db.QueryRow(`SELECT contrast || attempts || correct || updated_at FROM pair_stats`).Scan(&pairsBefore))
	db.Close()

	s, err := Open(path)
	must(t, err)
	defer s.Close()
	ctx := context.Background()

	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version < 6 || version != len(migrations) {
		t.Errorf("user_version = %d, want every migration (%d), 6 included", version, len(migrations))
	}
	if after := tableRows(t, s.db); !reflect.DeepEqual(after, before) {
		t.Errorf("rows changed by migration 6:\n%q\nwant\n%q", after, before)
	}
	var pairsAfter string
	must(t, s.db.QueryRow(`SELECT contrast || attempts || correct || updated_at FROM pair_stats`).Scan(&pairsAfter))
	if pairsAfter != pairsBefore {
		t.Errorf("pair_stats = %q, want %q", pairsAfter, pairsBefore)
	}
	// The old day reads back with 0 words read, and new data can be stored.
	days, err := s.Days(ctx)
	must(t, err)
	if len(days) != 1 || days[0].Minutes != 12 || days[0].WordsRead != 0 {
		t.Errorf("days = %+v, want the v5 day with 0 words read", days)
	}
	if _, err := s.SaveStory(ctx, sampleStory()); err != nil {
		t.Errorf("SaveStory after migration: %v", err)
	}
}

func TestMigration6OnAFreshDatabase(t *testing.T) {
	s := openTemp(t)
	var version, n int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version < 6 {
		t.Errorf("user_version = %d, want 6", version)
	}
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&n))
	if n != 0 {
		t.Errorf("stories has %d rows, want none", n)
	}
	must(t, s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('days') WHERE name = 'words_read'`).Scan(&n))
	if n != 1 {
		t.Error("days has no words_read column")
	}
}
