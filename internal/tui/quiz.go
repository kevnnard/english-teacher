package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// maxQuizMinutes caps the study time one quiz can record: ten quick
// questions take a few minutes, so a quiz left open does not fill the goal.
const maxQuizMinutes = 10

// quizMode is which quiz the learner takes (spec S11).
type quizMode int

const (
	quizVocab   quizMode = iota // from the learner's own cards, no AI
	quizGrammar                 // multiple-choice items from the teacher
)

// String is the mode as it reads in the activity log.
func (m quizMode) String() string {
	if m == quizGrammar {
		return "grammar"
	}
	return "vocabulary"
}

// quizPhase is where the learner is in a quiz.
type quizPhase int

const (
	quizPick     quizPhase = iota // choosing vocabulary or grammar
	quizLoading                   // waiting for the grammar items
	quizFailed                    // the grammar items failed; enter asks again
	quizAsking                    // a question waits for its answer
	quizAnswered                  // the feedback of the question on screen
	quizResult                    // every question is answered
)

// quizAnswer is what the learner answered to one question.
type quizAnswer struct {
	right  bool
	picked int          // multiple choice: the option chosen
	typed  string       // typed: what the learner wrote
	result check.Result // typed: the check of what the learner wrote
}

// quizState is the Quiz screen: one quiz, its answers and its result. A
// quiz is retrieval practice, not grading: it never changes a schedule.
type quizState struct {
	phase  quizPhase
	cursor quizMode // the mode under the pointer in the picker
	mode   quizMode
	req    int // the id of the latest grammar request; older answers are ignored
	count  int // quizzes started, mixed into the seed so each one differs

	topics    []string // grammar: what the items practice
	questions []quizQuestion
	answers   []quizAnswer // one per answered question, in order
	pos       int          // the question on screen
	input     string       // the typed answer so far
	startedAt time.Time    // when the first question appeared, for the study minutes

	notice string // why the chosen quiz cannot start
	err    error  // the grammar request failed
}

// typing reports whether the keys belong to a typed answer.
func (q quizState) typing() bool {
	return q.phase == quizAsking && q.questions[q.pos].typed()
}

// score is how many answers were right.
func (q quizState) score() int {
	n := 0
	for _, a := range q.answers {
		if a.right {
			n++
		}
	}
	return n
}

// quizGrammarMsg is the teacher's answer to grammar request req.
type quizGrammarMsg struct {
	req  int
	quiz teacher.GrammarQuiz
	err  error
}

// startQuiz starts the quiz of the mode under the pointer, or says why
// it cannot.
func (a App) startQuiz() (App, tea.Cmd) {
	q := &a.quiz
	q.notice = ""
	if q.cursor == quizGrammar {
		if a.teacher == nil {
			q.notice = "The AI teacher is off, so there is no grammar quiz. Install Gentle Shell (the pi command) and see `teacher models`."
			return a, nil
		}
		return a.askGrammar()
	}
	q.count++
	seed := uint64(a.now().UnixNano())
	questions := vocabQuiz(a.cards, rand.New(rand.NewPCG(seed, uint64(q.count))))
	if len(questions) == 0 {
		q.notice = "Your deck is empty. Add words in Learn, or import your vocabulary with `teacher import`."
		return a, nil
	}
	a.beginQuiz(quizVocab, nil, questions)
	return a, nil
}

// beginQuiz shows the first of questions.
func (a *App) beginQuiz(mode quizMode, topics []string, questions []quizQuestion) {
	q := &a.quiz
	a.quiz = quizState{
		phase: quizAsking, cursor: q.cursor, mode: mode, req: q.req, count: q.count,
		topics: topics, questions: questions, startedAt: a.now(),
	}
}

// askGrammar asks the teacher for grammar items on the learner's current
// lesson: the recurring error kinds, like Write's focus.
func (a App) askGrammar() (App, tea.Cmd) {
	q := &a.quiz
	q.req++
	q.phase, q.mode, q.err = quizLoading, quizGrammar, nil
	q.topics = teacher.GrammarTopics(a.profile.Level, a.focus())
	req, tc, level, focus := q.req, a.teacher, a.profile.Level, a.focus()
	return a, func() tea.Msg {
		quiz, err := tc.GrammarQuiz(context.Background(), level, focus)
		return quizGrammarMsg{req: req, quiz: quiz, err: err}
	}
}

// gotGrammar turns the teacher's items into questions, or shows the error.
func (a App) gotGrammar(msg quizGrammarMsg) App {
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
		questions[i] = quizQuestion{prompt: it.Question, options: it.Options, answer: it.Answer, word: it.Right(), why: it.Why}
	}
	a.beginQuiz(quizGrammar, msg.quiz.Topics, questions)
	return a
}

// answer records the learner's answer to the question on screen and shows
// its feedback. The last answer records the quiz, once.
func (a *App) answer(ans quizAnswer) {
	q := &a.quiz
	q.answers = append(q.answers, ans)
	q.phase = quizAnswered
	if len(q.answers) < len(q.questions) {
		return
	}
	minutes := min(int(a.now().Sub(q.startedAt)/time.Minute), maxQuizMinutes)
	activity := fmt.Sprintf("Quiz: %s %d/%d", q.mode, q.score(), len(q.questions))
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
}

func (a App) updateQuiz(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	q := &a.quiz
	k := msg.String()
	switch q.phase {
	case quizPick:
		switch k {
		case "up", "k":
			q.cursor, q.notice = quizVocab, ""
		case "down", "j":
			q.cursor, q.notice = quizGrammar, ""
		case "enter":
			return a.startQuiz()
		}
	case quizFailed:
		switch k {
		case "enter":
			return a.askGrammar()
		case "n":
			a.quiz = quizState{cursor: q.cursor, req: q.req, count: q.count}
		}
	case quizAsking:
		cur := q.questions[q.pos]
		if !cur.typed() {
			if n, ok := optionKey(k, len(cur.options)); ok {
				a.answer(quizAnswer{right: n == cur.answer, picked: n})
			}
			return a, nil
		}
		switch k {
		case "enter":
			res := check.Answer(q.input, cur.word)
			a.answer(quizAnswer{right: res.Kind == check.Exact, typed: q.input, result: res})
			q.input = ""
		case "esc":
			a.screen = Home // the quiz stays for later
		default:
			q.input = editInput(q.input, msg)
		}
	case quizAnswered:
		if k == "enter" {
			if q.pos+1 < len(q.questions) {
				q.pos++
				q.phase = quizAsking
			} else {
				q.phase = quizResult
			}
		}
	case quizResult:
		if k == "n" {
			a.quiz = quizState{cursor: q.cursor, req: q.req, count: q.count}
		}
	}
	return a, nil
}

// optionKey maps the keys 1 to n to an option index.
func optionKey(k string, n int) (int, bool) {
	if len(k) != 1 || k[0] < '1' || int(k[0]-'1') >= n {
		return 0, false
	}
	return int(k[0] - '1'), true
}
