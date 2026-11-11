package store

import (
	"context"
	"fmt"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// TickGoal records that the "can do" goal with id was proved on date. A
// goal already ticked keeps its first date.
func (s *Store) TickGoal(ctx context.Context, id string, date domain.Date) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO goal_ticks (goal, date) VALUES (?, ?) ON CONFLICT (goal) DO NOTHING`, id, date.String())
	if err != nil {
		return fmt.Errorf("tick goal %q: %w", id, err)
	}
	return nil
}

// GoalTicks returns the day each stored goal was ticked, by goal id.
func (s *Store) GoalTicks(ctx context.Context) (map[string]domain.Date, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT goal, date FROM goal_ticks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ticks := map[string]domain.Date{}
	for rows.Next() {
		var id, date string
		if err := rows.Scan(&id, &date); err != nil {
			return nil, err
		}
		d, err := domain.ParseDate(date)
		if err != nil {
			return nil, fmt.Errorf("goal %q: %w", id, err)
		}
		ticks[id] = d
	}
	return ticks, rows.Err()
}
