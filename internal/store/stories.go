package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// CurrentStory returns the story saved last, the one the learner is
// reading; ok is false when no story was ever saved.
func (s *Store) CurrentStory(ctx context.Context) (st domain.Story, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT id, title, chapter, summary, text, details, done FROM stories ORDER BY id DESC LIMIT 1`).
		Scan(&st.ID, &st.Title, &st.Chapter, &st.Summary, &st.Text, &st.Details, &st.Done)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Story{}, false, nil
	}
	if err != nil {
		return domain.Story{}, false, err
	}
	return st, true, nil
}

// SaveStory stores st and returns its id. A story with ID 0 is new and
// gets a new row; any other ID replaces that story's row, so a story
// keeps one row as its chapters go by.
func (s *Store) SaveStory(ctx context.Context, st domain.Story) (int64, error) {
	if st.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO stories (title, chapter, summary, text, details, done) VALUES (?, ?, ?, ?, ?, ?)`,
			st.Title, st.Chapter, st.Summary, st.Text, st.Details, st.Done)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE stories SET title = ?, chapter = ?, summary = ?, text = ?, details = ?, done = ? WHERE id = ?`,
		st.Title, st.Chapter, st.Summary, st.Text, st.Details, st.Done, st.ID)
	return st.ID, err
}
