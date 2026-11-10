package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// AddCards adds cards the teacher created, such as words marked in a
// reading or errors from a correction, and returns how many were new.
//
// Unlike UpsertCards it never changes a word that already exists: the
// learner's own meaning and schedule win. The one exception is a card's
// LastWrong, which is the learner's most recent mistake with that word: it
// is stored on the production direction, where the learner has to write the
// word and the margin note shows what they wrote before.
func (s *Store) AddCards(ctx context.Context, cards []domain.Card) (int, error) {
	added := 0
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // no-op after Commit

	for _, c := range cards {
		word := strings.TrimSpace(c.Word)
		if word == "" {
			continue
		}
		insert, err := tx.ExecContext(ctx,
			`INSERT INTO cards (word, pronunciation, meaning, example) VALUES (?, ?, ?, ?)
			ON CONFLICT (word) DO NOTHING`,
			word, c.Pronunciation, c.Meaning, c.Example)
		if err != nil {
			return 0, fmt.Errorf("add %q: %w", word, err)
		}
		if n, _ := insert.RowsAffected(); n > 0 {
			added++
		}
		if c.LastWrong == "" {
			continue
		}
		// Only last_wrong is set on conflict, so an existing schedule stays.
		_, err = tx.ExecContext(ctx,
			`INSERT INTO reviews (card_id, direction, ease_factor, last_wrong)
			SELECT id, ?, ?, ? FROM cards WHERE word = ?
			ON CONFLICT (card_id, direction) DO UPDATE SET last_wrong = excluded.last_wrong`,
			domain.Production.String(), domain.DefaultEaseFactor, c.LastWrong, word)
		if err != nil {
			return 0, fmt.Errorf("add %q: %w", word, err)
		}
	}
	return added, tx.Commit()
}
