package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// The steps after the correction: the learner's own fixes (3), the
// feedback with its cards and error log (4) and the rewrite (5).

// submitAttempt keeps the learner's fix of the current error. After the
// last one, exact fixes count as fixed; the others go to the judge in one
// call.
func (a App) submitAttempt() (App, tea.Cmd) {
	w := &a.write
	w.majors[w.cursor].attempt = strings.TrimSpace(w.input)
	w.input = ""
	w.cursor++
	if w.cursor < len(w.majors) {
		return a, nil
	}

	w.judged = nil
	var attempts []teacher.Attempt
	for i := range w.majors {
		m := &w.majors[i]
		switch {
		case m.attempt == "":
			// "I don't know" is not fixed.
		case check.Answer(m.attempt, m.Right).Kind == check.Exact:
			m.fixed = true
		default:
			w.judged = append(w.judged, i)
			attempts = append(attempts, teacher.Attempt{
				Sentence: sentenceWith(w.text, m.Wrong), Wrong: m.Wrong, Right: m.Right, Attempt: m.attempt,
			})
		}
	}
	if len(attempts) == 0 {
		a.startFeedback()
		return a, nil
	}
	w.req++
	w.phase = writeJudging
	req, tc, level := w.req, a.teacher, a.studyLevel()
	return a, func() tea.Msg {
		v, err := tc.Judge(context.Background(), level, attempts)
		return judgeMsg{req: req, verdicts: v, err: err}
	}
}

// gotJudge marks the judged attempts and shows the feedback. When the
// judge fails, the judged attempts stay not fixed.
func (a App) gotJudge(msg judgeMsg) App {
	w := &a.write
	if msg.req != w.req || w.phase != writeJudging {
		return a
	}
	err := msg.err
	if err == nil && len(msg.verdicts) != len(w.judged) {
		err = fmt.Errorf("the judge answered %d verdicts for %d fixes", len(msg.verdicts), len(w.judged))
	}
	if err != nil {
		w.judgeErr = err
	} else {
		for j, i := range w.judged {
			w.majors[i].fixed = msg.verdicts[j]
		}
	}
	a.startFeedback()
	return a
}

// startFeedback turns the errors the learner did not fix into cards and
// logs every major error.
func (a *App) startFeedback() {
	w := &a.write
	w.phase = writeFeedback
	w.cards, w.rewrites = nil, nil
	for i, m := range w.majors {
		if !m.fixed {
			w.cards = append(w.cards, m.Card(sentenceWith(w.text, m.Wrong)))
			w.rewrites = append(w.rewrites, i)
		}
	}
	a.saveFixCards()
	a.logMistakes()
}

// logMistakes logs the major errors and finds the pattern line: the most
// repeated kind of this text, counting what was logged before. The
// in-memory counts change only when the log is stored.
func (a *App) logMistakes() {
	w := &a.write
	if len(w.majors) == 0 {
		return
	}
	mistakes := make([]domain.Mistake, len(w.majors))
	inText := map[string]int{}
	for i, m := range w.majors {
		kind := teacher.NormalizeKind(m.Kind)
		mistakes[i] = domain.Mistake{Date: a.today(), Kind: kind, Wrong: m.Wrong, Right: m.Right}
		inText[kind]++
	}
	for _, m := range mistakes {
		seen := a.mistakeCounts[m.Kind] + inText[m.Kind]
		if m.Kind != teacher.OtherKind && seen >= recurringCount && seen > w.patternCount {
			w.patternKind, w.patternCount = m.Kind, seen
		}
	}

	w.logErr = a.storeMistakes(mistakes)
}

// storeMistakes logs mistakes and, only once they are stored, adds them
// to the in-memory counts that choose the next writing focus.
func (a *App) storeMistakes(mistakes []domain.Mistake) error {
	if a.recordMistakes != nil {
		if err := a.recordMistakes(mistakes); err != nil {
			return err
		}
	}
	if a.mistakeCounts == nil {
		a.mistakeCounts = map[string]int{}
	}
	for _, m := range mistakes {
		a.mistakeCounts[m.Kind]++
	}
	return nil
}

// checkRewrite checks the learner's rewrite of the current error locally.
func (w *writeState) checkRewrite() {
	m := &w.majors[w.rewrites[w.cursor]]
	m.rewrite = check.Answer(w.input, m.Right)
	w.checked = true
}

// nextRewrite moves to the next error to rewrite, or to the result.
func (w *writeState) nextRewrite() {
	w.cursor++
	w.input, w.checked = "", false
	if w.cursor >= len(w.rewrites) {
		w.phase = writeResult
	}
}

// ordinal writes n as "1st", "2nd", "3rd", "4th"…
func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
