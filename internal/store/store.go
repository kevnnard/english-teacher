// Package store persists cards in SQLite using the pure-Go modernc.org/sqlite driver.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// ErrNotFound is returned when a card's word is not in the store.
var ErrNotFound = errors.New("card not found")

// timeLayout is how timestamps are stored as TEXT.
const timeLayout = time.RFC3339Nano

// migrations run in order; the database remembers how many it has applied
// in PRAGMA user_version. Only ever append to this list.
var migrations = []string{
	`CREATE TABLE cards (
		id             INTEGER PRIMARY KEY,
		word           TEXT    NOT NULL UNIQUE COLLATE NOCASE,
		pronunciation  TEXT    NOT NULL DEFAULT '',
		meaning        TEXT    NOT NULL DEFAULT '',
		example        TEXT    NOT NULL DEFAULT '',
		times_correct  INTEGER NOT NULL DEFAULT 0,
		times_failed   INTEGER NOT NULL DEFAULT 0,
		last_reviewed  TEXT,
		ease_factor    REAL    NOT NULL DEFAULT 2.5,
		interval_days  INTEGER NOT NULL DEFAULT 0,
		repetitions    INTEGER NOT NULL DEFAULT 0,
		due            TEXT    NOT NULL DEFAULT ''
	)`,

	// 2: each word is studied in two directions with separate schedules.
	// Existing progress becomes the recognition direction; production rows
	// are created on the first save.
	`CREATE TABLE reviews (
		card_id        INTEGER NOT NULL REFERENCES cards(id),
		direction      TEXT    NOT NULL,
		times_correct  INTEGER NOT NULL DEFAULT 0,
		times_failed   INTEGER NOT NULL DEFAULT 0,
		last_reviewed  TEXT,
		ease_factor    REAL    NOT NULL DEFAULT 2.5,
		interval_days  INTEGER NOT NULL DEFAULT 0,
		repetitions    INTEGER NOT NULL DEFAULT 0,
		due            TEXT    NOT NULL DEFAULT '',
		last_wrong     TEXT    NOT NULL DEFAULT '',
		PRIMARY KEY (card_id, direction)
	);
	INSERT INTO reviews (card_id, direction, times_correct, times_failed, last_reviewed,
		ease_factor, interval_days, repetitions, due)
	SELECT id, 'recognition', times_correct, times_failed, last_reviewed,
		ease_factor, interval_days, repetitions, due
	FROM cards;
	ALTER TABLE cards DROP COLUMN times_correct;
	ALTER TABLE cards DROP COLUMN times_failed;
	ALTER TABLE cards DROP COLUMN last_reviewed;
	ALTER TABLE cards DROP COLUMN ease_factor;
	ALTER TABLE cards DROP COLUMN interval_days;
	ALTER TABLE cards DROP COLUMN repetitions;
	ALTER TABLE cards DROP COLUMN due;`,
}

// Store is a SQLite-backed card repository.
type Store struct {
	db *sql.DB
}

// ImportResult counts what UpsertCards did.
type ImportResult struct {
	Added   int
	Updated int
}

// Open opens (or creates) the database at path and applies pending migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps SQLite simple: no "database is locked" between our own queries.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept placeholders; i+1 is our own integer.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// UpsertCards inserts new cards and refreshes the text of existing ones.
//
// Words are matched case-insensitively. For a card that already exists only
// pronunciation, meaning and example are updated: its counters, last review
// and SM-2 schedules belong to this app and are never reset by an import.
// A new card's counters and last review become its recognition progress.
func (s *Store) UpsertCards(ctx context.Context, cards []domain.Card) (ImportResult, error) {
	var res ImportResult
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback() // no-op after Commit

	for _, c := range cards {
		update, err := tx.ExecContext(ctx,
			`UPDATE cards SET pronunciation = ?, meaning = ?, example = ? WHERE word = ?`,
			c.Pronunciation, c.Meaning, c.Example, c.Word)
		if err != nil {
			return res, fmt.Errorf("update %q: %w", c.Word, err)
		}
		if n, _ := update.RowsAffected(); n > 0 {
			res.Updated++
			continue
		}

		insert, err := tx.ExecContext(ctx,
			`INSERT INTO cards (word, pronunciation, meaning, example) VALUES (?, ?, ?, ?)`,
			c.Word, c.Pronunciation, c.Meaning, c.Example)
		if err != nil {
			return res, fmt.Errorf("insert %q: %w", c.Word, err)
		}
		id, err := insert.LastInsertId()
		if err != nil {
			return res, err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO reviews (card_id, direction, times_correct, times_failed, last_reviewed,
				ease_factor, interval_days, repetitions, due)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, domain.Recognition.String(), c.TimesCorrect, c.TimesFailed, formatOptional(c.LastReviewed),
			c.EaseFactor, c.IntervalDays, c.Repetitions, formatTime(c.Due))
		if err != nil {
			return res, fmt.Errorf("insert %q: %w", c.Word, err)
		}
		res.Added++
	}
	return res, tx.Commit()
}

// ListCards returns every word studied in direction dir, in the order the
// words were first imported. A word never reviewed in dir comes back with a
// fresh schedule.
func (s *Store) ListCards(ctx context.Context, dir domain.Direction) ([]domain.Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.word, c.pronunciation, c.meaning, c.example,
			COALESCE(r.times_correct, 0), COALESCE(r.times_failed, 0), r.last_reviewed,
			COALESCE(r.ease_factor, ?), COALESCE(r.interval_days, 0), COALESCE(r.repetitions, 0),
			COALESCE(r.due, ''), COALESCE(r.last_wrong, '')
		FROM cards c
		LEFT JOIN reviews r ON r.card_id = c.id AND r.direction = ?
		ORDER BY c.id`,
		domain.DefaultEaseFactor, dir.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []domain.Card
	for rows.Next() {
		var (
			c            = domain.Card{Direction: dir}
			lastReviewed sql.NullString
			due          string
		)
		err := rows.Scan(&c.Word, &c.Pronunciation, &c.Meaning, &c.Example, &c.TimesCorrect, &c.TimesFailed,
			&lastReviewed, &c.EaseFactor, &c.IntervalDays, &c.Repetitions, &due, &c.LastWrong)
		if err != nil {
			return nil, err
		}
		if c.Due, err = parseTime(due); err != nil {
			return nil, fmt.Errorf("card %q: due: %w", c.Word, err)
		}
		if lastReviewed.Valid {
			t, err := parseTime(lastReviewed.String)
			if err != nil {
				return nil, fmt.Errorf("card %q: last_reviewed: %w", c.Word, err)
			}
			c.LastReviewed = &t
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// SaveCard stores the counters, schedule and last wrong answer of c in its
// direction after a review. It returns ErrNotFound if c.Word is not in the store.
func (s *Store) SaveCard(ctx context.Context, c domain.Card) error {
	// The WHERE clause also tells SQLite the ON CONFLICT belongs to the INSERT.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reviews (card_id, direction, times_correct, times_failed, last_reviewed,
			ease_factor, interval_days, repetitions, due, last_wrong)
		SELECT id, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM cards WHERE word = ?
		ON CONFLICT (card_id, direction) DO UPDATE SET
			times_correct = excluded.times_correct, times_failed = excluded.times_failed,
			last_reviewed = excluded.last_reviewed, ease_factor = excluded.ease_factor,
			interval_days = excluded.interval_days, repetitions = excluded.repetitions,
			due = excluded.due, last_wrong = excluded.last_wrong`,
		c.Direction.String(), c.TimesCorrect, c.TimesFailed, formatOptional(c.LastReviewed),
		c.EaseFactor, c.IntervalDays, c.Repetitions, formatTime(c.Due), c.LastWrong, c.Word)
	if err != nil {
		return fmt.Errorf("save %q: %w", c.Word, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("save %q: %w", c.Word, ErrNotFound)
	}
	return nil
}

// formatTime stores the zero time as "" so it reads back as zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(timeLayout, s)
}

// formatOptional maps a nil time to SQL NULL.
func formatOptional(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(timeLayout)
}
