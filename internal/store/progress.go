package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Days returns every recorded study day, oldest first.
func (s *Store) Days(ctx context.Context) ([]domain.Day, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, minutes, reviews, correct, failed, words_read, activities FROM days ORDER BY date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []domain.Day
	for rows.Next() {
		d, err := scanDay(rows)
		if err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// RecordStudy adds delta to its day: minutes and counts are summed, and
// activities not done yet that day are appended. A negative delta takes
// back an undone grade; counts never go below zero.
func (s *Store) RecordStudy(ctx context.Context, delta domain.Day) error {
	return s.updateDay(ctx, delta.Date, func(d *domain.Day) {
		d.Minutes = max(0, d.Minutes+delta.Minutes)
		d.Reviews = max(0, d.Reviews+delta.Reviews)
		d.Correct = max(0, d.Correct+delta.Correct)
		d.Failed = max(0, d.Failed+delta.Failed)
		d.WordsRead = max(0, d.WordsRead+delta.WordsRead)
		d.Activities = mergeActivities(d.Activities, delta.Activities)
	})
}

// MergeDays folds imported history into the stored days and returns how
// many days were new. It is idempotent: counts take the larger of the two
// values instead of adding up, and activities already there are skipped,
// so importing the same history twice changes nothing.
func (s *Store) MergeDays(ctx context.Context, history []domain.Day) (int, error) {
	added := 0
	for _, h := range history {
		existed, err := s.dayExists(ctx, h.Date)
		if err != nil {
			return added, err
		}
		err = s.updateDay(ctx, h.Date, func(d *domain.Day) {
			d.Minutes = max(d.Minutes, h.Minutes)
			d.Reviews = max(d.Reviews, h.Reviews)
			d.Correct = max(d.Correct, h.Correct)
			d.Failed = max(d.Failed, h.Failed)
			d.WordsRead = max(d.WordsRead, h.WordsRead)
			d.Activities = mergeActivities(d.Activities, h.Activities)
		})
		if err != nil {
			return added, err
		}
		if !existed {
			added++
		}
	}
	return added, nil
}

// Profile returns the learner profile, or domain.NewProfile when none is saved.
func (s *Store) Profile(ctx context.Context) (domain.Profile, error) {
	var (
		p       domain.Profile
		level   string
		started string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT name, level, started, daily_goal_minutes FROM profile WHERE id = 1`).
		Scan(&p.Name, &level, &started, &p.DailyGoalMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewProfile(), nil
	}
	if err != nil {
		return p, err
	}
	if p.Level, err = domain.ParseLevel(level); err != nil {
		return p, fmt.Errorf("profile: %w", err)
	}
	if started != "" {
		if p.Started, err = domain.ParseDate(started); err != nil {
			return p, fmt.Errorf("profile: started: %w", err)
		}
	}
	return p, nil
}

// SaveProfile stores p, replacing the saved profile.
func (s *Store) SaveProfile(ctx context.Context, p domain.Profile) error {
	started := ""
	if p.Started != (domain.Date{}) {
		started = p.Started.String()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile (id, name, level, started, daily_goal_minutes) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, level = excluded.level,
			started = excluded.started, daily_goal_minutes = excluded.daily_goal_minutes`,
		p.Name, p.Level.String(), started, p.DailyGoalMinutes)
	return err
}

// updateDay reads the day of date (empty when missing), lets change edit
// it and writes it back, all in one transaction.
func (s *Store) updateDay(ctx context.Context, date domain.Date, change func(*domain.Day)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	d, err := scanDay(tx.QueryRowContext(ctx,
		`SELECT date, minutes, reviews, correct, failed, words_read, activities FROM days WHERE date = ?`, date.String()))
	if errors.Is(err, sql.ErrNoRows) {
		d, err = domain.Day{Date: date}, nil
	}
	if err != nil {
		return err
	}
	change(&d)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO days (date, minutes, reviews, correct, failed, words_read, activities) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (date) DO UPDATE SET minutes = excluded.minutes, reviews = excluded.reviews,
			correct = excluded.correct, failed = excluded.failed, words_read = excluded.words_read,
			activities = excluded.activities`,
		date.String(), d.Minutes, d.Reviews, d.Correct, d.Failed, d.WordsRead, strings.Join(d.Activities, "\n"))
	if err != nil {
		return fmt.Errorf("day %s: %w", date, err)
	}
	return tx.Commit()
}

func (s *Store) dayExists(ctx context.Context, date domain.Date) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM days WHERE date = ?`, date.String()).Scan(&n)
	return n > 0, err
}

// scanner is what *sql.Row and *sql.Rows have in common.
type scanner interface {
	Scan(dest ...any) error
}

func scanDay(row scanner) (domain.Day, error) {
	var (
		d          domain.Day
		date, acts string
	)
	if err := row.Scan(&date, &d.Minutes, &d.Reviews, &d.Correct, &d.Failed, &d.WordsRead, &acts); err != nil {
		return d, err
	}
	var err error
	if d.Date, err = domain.ParseDate(date); err != nil {
		return d, err
	}
	if acts != "" {
		d.Activities = strings.Split(acts, "\n")
	}
	return d, nil
}

// mergeActivities appends the activities of more that have not been done
// yet, keeping order. Newlines would break storage, so they become spaces.
func mergeActivities(done, more []string) []string {
	for _, a := range more {
		a = strings.TrimSpace(strings.ReplaceAll(a, "\n", " "))
		if a != "" && !slices.Contains(done, a) {
			done = append(done, a)
		}
	}
	return done
}
