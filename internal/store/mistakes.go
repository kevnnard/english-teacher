package store

import (
	"context"
	"fmt"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// RecordMistakes logs every mistake in one transaction. Nothing is
// written when one of them fails.
func (s *Store) RecordMistakes(ctx context.Context, mistakes []domain.Mistake) error {
	if len(mistakes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	for _, m := range mistakes {
		_, err := tx.ExecContext(ctx, `INSERT INTO mistakes (date, kind, wrong, right) VALUES (?, ?, ?, ?)`,
			m.Date.String(), m.Kind, m.Wrong, m.Right)
		if err != nil {
			return fmt.Errorf("record mistake %q: %w", m.Wrong, err)
		}
	}
	return tx.Commit()
}

// MistakeCounts returns how many mistakes of each kind were ever logged.
// Kinds never seen are absent from the map.
func (s *Store) MistakeCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind, COUNT(*) FROM mistakes GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var (
			kind string
			n    int
		)
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		counts[kind] = n
	}
	return counts, rows.Err()
}
