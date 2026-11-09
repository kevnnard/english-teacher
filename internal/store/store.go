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
// and SM-2 schedule belong to this app and are never reset by an import.
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

		_, err = tx.ExecContext(ctx,
			`INSERT INTO cards (word, pronunciation, meaning, example, times_correct, times_failed,
				last_reviewed, ease_factor, interval_days, repetitions, due)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Word, c.Pronunciation, c.Meaning, c.Example, c.TimesCorrect, c.TimesFailed,
			formatOptional(c.LastReviewed), c.EaseFactor, c.IntervalDays, c.Repetitions, formatTime(c.Due))
		if err != nil {
			return res, fmt.Errorf("insert %q: %w", c.Word, err)
		}
		res.Added++
	}
	return res, tx.Commit()
}

// ListCards returns every card in the order it was first imported.
func (s *Store) ListCards(ctx context.Context) ([]domain.Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT word, pronunciation, meaning, example, times_correct, times_failed,
			last_reviewed, ease_factor, interval_days, repetitions, due
		FROM cards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []domain.Card
	for rows.Next() {
		var (
			c            domain.Card
			lastReviewed sql.NullString
			due          string
		)
		err := rows.Scan(&c.Word, &c.Pronunciation, &c.Meaning, &c.Example, &c.TimesCorrect, &c.TimesFailed,
			&lastReviewed, &c.EaseFactor, &c.IntervalDays, &c.Repetitions, &due)
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

// SaveCard stores the counters and schedule of c after a review.
// It returns ErrNotFound if c.Word is not in the store.
func (s *Store) SaveCard(ctx context.Context, c domain.Card) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE cards SET times_correct = ?, times_failed = ?, last_reviewed = ?,
			ease_factor = ?, interval_days = ?, repetitions = ?, due = ?
		WHERE word = ?`,
		c.TimesCorrect, c.TimesFailed, formatOptional(c.LastReviewed),
		c.EaseFactor, c.IntervalDays, c.Repetitions, formatTime(c.Due), c.Word)
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
