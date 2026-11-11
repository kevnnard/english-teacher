package store

import (
	"context"
	"fmt"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// RecordPairResults adds the results of one minimal-pairs round to the
// totals of each contrast, in one transaction, and marks them as updated
// on date. Nothing is written when one of them fails.
func (s *Store) RecordPairResults(ctx context.Context, date domain.Date, results []domain.PairStat) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	for _, r := range results {
		_, err := tx.ExecContext(ctx, `INSERT INTO pair_stats (contrast, attempts, correct, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (contrast) DO UPDATE SET
				attempts   = attempts + excluded.attempts,
				correct    = correct + excluded.correct,
				updated_at = excluded.updated_at`,
			r.Contrast, r.Attempts, r.Correct, date.String())
		if err != nil {
			return fmt.Errorf("record pair results of %q: %w", r.Contrast, err)
		}
	}
	return tx.Commit()
}

// PairStats returns the totals of every contrast the learner has heard,
// by contrast ID. Contrasts never heard are absent from the map.
func (s *Store) PairStats(ctx context.Context) (map[string]domain.PairStat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT contrast, attempts, correct FROM pair_stats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]domain.PairStat{}
	for rows.Next() {
		var st domain.PairStat
		if err := rows.Scan(&st.Contrast, &st.Attempts, &st.Correct); err != nil {
			return nil, err
		}
		stats[st.Contrast] = st
	}
	return stats, rows.Err()
}
