package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// Grammar in context (spec S4): a cloze round in Quiz. The teacher gaps
// the learner's own sentences, from the current story chapter and the card
// examples, and writes new ones when those do not fit. The learner types
// each gap; a chunk shown with an item can become a Review card.

// quizClozeMsg is the teacher's answer to cloze request req.
type quizClozeMsg struct {
	req  int
	quiz teacher.ClozeQuiz
	err  error
}

// askCloze asks the teacher for a cloze round on the learner's recurring
// error kinds and the core grammar of the level.
func (a App) askCloze() (App, tea.Cmd) {
	q := &a.quiz
	q.req++
	q.phase, q.mode, q.err = quizLoading, quizCloze, nil
	q.topics = teacher.ClozePoints(a.studyLevel(), a.recurringKinds())
	req, tc, level, mistakes := q.req, a.teacher, a.studyLevel(), a.recurringKinds()
	sentences := teacher.OwnSentences(a.ownTexts())
	return a, func() tea.Msg {
		quiz, err := tc.ClozeRound(context.Background(), level, mistakes, sentences)
		return quizClozeMsg{req: req, quiz: quiz, err: err}
	}
}

// ownTexts are the learner's own English texts for a cloze round: the
// story chapter in Learn, then the card examples, newest card first.
func (a App) ownTexts() []string {
	var texts []string
	if s := a.story.saved; s.Title != "" {
		texts = append(texts, s.Text)
	}
	for _, c := range slices.Backward(a.cards) {
		texts = append(texts, c.Example)
	}
	return texts
}

// gotCloze turns the teacher's items into typed questions, or shows the error.
func (a App) gotCloze(msg quizClozeMsg) App {
	q := &a.quiz
	if msg.req != q.req || q.phase != quizLoading {
		return a
	}
	if msg.err != nil {
		q.phase, q.err = quizFailed, msg.err
		return a
	}
	questions := make([]quizQuestion, len(msg.quiz.Items))
	for i, it := range msg.quiz.Items {
		gapped, _ := check.Gap(it.Sentence, it.Answer) // the teacher keeps only items that gap
		questions[i] = quizQuestion{
			prompt: gapped, word: it.Answer, why: it.Rule,
			hint: it.Hint, point: it.Point, chunk: it.Chunk, yours: it.Yours,
		}
	}
	a.beginQuiz(quizCloze, msg.quiz.Points, questions)
	return a
}

// inDeck reports whether the deck or this round already has phrase.
func (a App) inDeck(phrase string) bool {
	match := func(w string) bool { return strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(phrase)) }
	return slices.ContainsFunc(a.quiz.added, match) || slices.ContainsFunc(a.cards, func(c domain.Card) bool { return match(c.Word) })
}

// addChunk adds c to the deck as a Review card, unless it is there
// already, and says what happened.
func (a *App) addChunk(c domain.Chunk) string {
	if a.inDeck(c.Phrase) {
		return "already in your cards"
	}
	if err := a.addChunkCards([]domain.Chunk{c}); err != nil {
		return "✗ could not add the chunk: " + err.Error()
	}
	return "✓ added to Review"
}

// addMissedChunks adds the chunks of the round's missed items that are not
// in the deck yet, once the round ends.
func (a *App) addMissedChunks() {
	q := &a.quiz
	var chunks []domain.Chunk
	for i, ans := range q.answers {
		c := q.questions[i].chunk
		if ans.right || !c.Valid() || a.inDeck(c.Phrase) ||
			slices.ContainsFunc(chunks, func(d domain.Chunk) bool { return strings.EqualFold(d.Phrase, c.Phrase) }) {
			continue
		}
		chunks = append(chunks, c)
	}
	if len(chunks) == 0 {
		return
	}
	if err := a.addChunkCards(chunks); err != nil {
		q.missedErr = err
		return
	}
	for _, c := range chunks {
		q.missed = append(q.missed, c.Phrase)
	}
}

// addChunkCards stores chunks as cards through the same path as Learn's
// glossary: the store skips a word it has, and the deck gets both
// directions of each new one.
func (a *App) addChunkCards(chunks []domain.Chunk) error {
	if a.add == nil {
		return fmt.Errorf("cards cannot be saved here")
	}
	cards := make([]domain.Card, len(chunks))
	for i, c := range chunks {
		cards[i] = c.Card()
	}
	if _, err := a.add(cards); err != nil {
		return err
	}
	a.addCards(cards)
	for _, c := range chunks {
		a.quiz.added = append(a.quiz.added, c.Phrase)
	}
	return nil
}
