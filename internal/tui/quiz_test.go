package tui

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// quizEnv is the Quiz screen with fakes, plus the card saver, so tests can
// check a quiz never touches the schedules.
type quizEnv struct {
	aiApp
	rec   *recorder
	cards []domain.Card // the deck the app started with
}

// newQuizApp opens Quiz over cards with the AI teacher's fakes and the
// error kinds already logged.
func newQuizApp(t *testing.T, cards []domain.Card, counts map[string]int) quizEnv {
	t.Helper()
	x := quizEnv{
		aiApp: aiApp{ft: &fakeTeacher{}, ad: &adder{}, log: &studyLog{}, mk: &mistakeLog{}, clock: &testClock{t: fixedNow}},
		rec:   &recorder{}, cards: cards,
	}
	a := NewApp(Config{
		Cards: cards, Save: x.rec.save, Now: x.clock.now, Start: Quiz, MaxNew: DefaultMaxNew,
		Profile: domain.Profile{Level: domain.A2}, Record: x.log.record,
		Teacher: x.ft, Add: x.ad.add, MistakeCounts: counts, RecordMistakes: x.mk.record,
	})
	x.App = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// untouched checks the quiz saved no card and changed no schedule.
func (x quizEnv) untouched(t *testing.T, a App) {
	t.Helper()
	if len(x.rec.saved) != 0 {
		t.Errorf("the quiz saved %d cards", len(x.rec.saved))
	}
	if !reflect.DeepEqual(a.cards, x.cards) {
		t.Error("the quiz changed the cards in memory")
	}
	if a.review.remaining() != x.App.review.remaining() {
		t.Errorf("due %d, want %d", a.review.remaining(), x.App.review.remaining())
	}
}

// question is the question on screen.
func question(a App) quizQuestion { return a.quiz.questions[a.quiz.pos] }

// answerRight answers the question on screen correctly.
func answerRight(t *testing.T, a App) App {
	t.Helper()
	q := question(a)
	if q.typed() {
		a = typeText(t, a, q.word)
		a, _ = press(t, a, "enter")
		return a
	}
	a, _ = press(t, a, fmt.Sprint(q.answer+1))
	return a
}

// wrongOption is a key 1–4 that picks a wrong option.
func wrongOption(q quizQuestion) string {
	return fmt.Sprint((q.answer+1)%len(q.options) + 1)
}

func TestQuizPickerShowsBothModes(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	mustContain(t, pageText(x.App), "Quiz", "never changes your review dates",
		"▸ Vocabulary", "from your cards", "Grammar")
	mustContain(t, statusBar(x.App), "↑↓ choose", "enter start")

	a, _ := press(t, x.App, "down")
	mustContain(t, pageText(a), "▸ Grammar")
	a, _ = press(t, a, "k")
	mustContain(t, pageText(a), "▸ Vocabulary")
}

func TestVocabularyQuizRunsTenQuestionsAndRecordsTheResult(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	a, _ := press(t, x.App, "enter")
	if len(a.quiz.questions) != vocabQuestions {
		t.Fatalf("%d questions", len(a.quiz.questions))
	}
	mustContain(t, pageText(a), "Quiz · Vocabulary", "Question 1 of 10")

	var wrongMC, almost quizQuestion
	for i := range vocabQuestions {
		q := question(a)
		mustContain(t, pageText(a), fmt.Sprintf("Question %d of 10", i+1), q.prompt)
		switch {
		case !q.typed() && wrongMC.word == "":
			mustContain(t, pageText(a), "Which English word means", "1  "+q.options[0], "4  "+q.options[3])
			mustContain(t, statusBar(a), "1–4 choose")
			wrongMC = q
			a, _ = press(t, a, wrongOption(q))
			mustContain(t, pageText(a), "✗", "the answer is "+q.word)
		case q.typed() && almost.word == "":
			mustContain(t, pageText(a), "Type the English", `"I say ___`, `_ every day."`)
			mustContain(t, statusBar(a), "enter check")
			almost = q
			a = typeText(t, a, q.word+"x")
			a, _ = press(t, a, "enter")
			mustContain(t, pageText(a), "✗ almost", "[x→]", "the answer is "+q.word)
		default:
			a = answerRight(t, a)
			mustContain(t, pageText(a), "✓ correct")
		}
		if i < vocabQuestions-1 {
			mustContain(t, statusBar(a), "enter next")
			if len(x.log.deltas) != 0 {
				t.Fatalf("recorded %v before the last answer", x.log.deltas)
			}
			x.clock.t = x.clock.t.Add(20 * time.Second)
		}
		a, _ = press(t, a, "enter")
	}

	page := pageText(a)
	mustContain(t, page, "8/10", "To review",
		wrongMC.prompt+" → "+wrongMC.word, almost.prompt+" → "+almost.word, "you wrote "+almost.word+"x")
	mustContain(t, statusBar(a), "n new quiz")

	// 9 × 20 s is 3 whole minutes.
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 3, Activities: []string{"Quiz: vocabulary 8/10"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	x.untouched(t, a)

	a, _ = press(t, a, "enter", "1", "x")
	if len(x.log.deltas) != 1 {
		t.Errorf("recorded again: %v", x.log.deltas)
	}
	a, _ = press(t, a, "n")
	mustContain(t, pageText(a), "▸ Vocabulary")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "Question 1 of 10")
	if len(x.log.deltas) != 1 {
		t.Errorf("a new quiz recorded something: %v", x.log.deltas)
	}
}

func TestVocabularyQuizWrongTypedAnswerShowsTheAnswer(t *testing.T) {
	x := newQuizApp(t, quizDeck()[:6], nil) // typed only
	a, _ := press(t, x.App, "enter")
	q := question(a)
	a = typeText(t, a, "banana")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "✗ not quite", "the answer is "+q.word)

	// An empty answer is "I don't know".
	a, _ = press(t, a, "enter", "enter")
	mustContain(t, pageText(a), "✗ not quite")
	a, _ = press(t, a, "enter")
	a = answerRight(t, a)
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "1/3", "you wrote banana", "you wrote nothing")
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 0, Activities: []string{"Quiz: vocabulary 1/3"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	x.untouched(t, a)
}

func TestVocabularyQuizMinutesAreCappedAtTen(t *testing.T) {
	x := newQuizApp(t, quizDeck()[:6], nil)
	a, _ := press(t, x.App, "enter")
	for range 3 {
		a = answerRight(t, a)
		x.clock.t = x.clock.t.Add(30 * time.Minute) // the learner walked away
		a, _ = press(t, a, "enter")
	}
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: maxQuizMinutes, Activities: []string{"Quiz: vocabulary 3/3"}}}
	if maxQuizMinutes != 10 || !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
}

func TestVocabularyQuizWithAFewCardsIsTypedOnly(t *testing.T) {
	x := newQuizApp(t, quizDeck()[:6], nil)
	a, _ := press(t, x.App, "enter")
	mustContain(t, pageText(a), "Question 1 of 3", "Type the English")
	for _, q := range a.quiz.questions {
		if !q.typed() {
			t.Errorf("%q is multiple choice", q.word)
		}
	}
}

func TestVocabularyQuizWithAnEmptyDeckSaysSo(t *testing.T) {
	x := newQuizApp(t, nil, nil)
	a, _ := press(t, x.App, "enter")
	mustContain(t, pageText(a), "Your deck is empty", "▸ Vocabulary")
	if a.quiz.phase != quizPick || len(x.log.deltas) != 0 {
		t.Errorf("phase %v, deltas %v", a.quiz.phase, x.log.deltas)
	}
}

func TestVocabularyQuizWorksWithoutTheTeacher(t *testing.T) {
	a := newApp(t, quizDeck(), &recorder{}, Quiz, 120, 30)
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "Question 1 of 10")

	// The grammar quiz needs the teacher.
	a = newApp(t, quizDeck(), &recorder{}, Quiz, 120, 30)
	a, cmd := press(t, a, "down", "enter")
	if cmd != nil || a.quiz.phase != quizPick {
		t.Errorf("phase %v, cmd %v: the grammar quiz started without a teacher", a.quiz.phase, cmd != nil)
	}
	mustContain(t, pageText(a), "The AI teacher is off")
}

func TestQuizTypedItemsTakeEveryKeyAsALetter(t *testing.T) {
	x := newQuizApp(t, quizDeck()[:6], nil)
	a, _ := press(t, x.App, "enter")
	a = typeText(t, a, "1q?:4n")
	a, _ = press(t, a, "tab", "shift+tab", "backspace")
	if a.Screen() != Quiz || a.help || a.palette.open {
		t.Fatalf("screen %v, help %v, palette %v: keys must be letters", a.Screen(), a.help, a.palette.open)
	}
	if a.quiz.input != "1q?:4" {
		t.Errorf("input = %q", a.quiz.input)
	}
	mustContain(t, pageText(a), "› 1q?:4")
}

func TestQuizMultipleChoiceTakesOnlyOneToFour(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	a, _ := press(t, x.App, "enter")
	if question(a).typed() {
		t.Fatal("the first question must be multiple choice")
	}
	a, _ = press(t, a, "5", "0", "x", "enter")
	if a.quiz.phase != quizAsking || len(a.quiz.answers) != 0 {
		t.Errorf("phase %v, answers %v", a.quiz.phase, a.quiz.answers)
	}
}

func TestQuizEarlyQuitRecordsNothingAndEscKeepsTheQuiz(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	a, _ := press(t, x.App, "enter")
	for range 3 {
		a = answerRight(t, a)
		a, _ = press(t, a, "enter")
	}
	// esc during a typed item goes Home and keeps what was typed.
	if !question(a).typed() {
		t.Fatal("question 4 must be typed")
	}
	a = typeText(t, a, "ho")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Fatalf("esc went to %v", a.Screen())
	}
	a, _ = press(t, a, "shift+tab", "shift+tab", "shift+tab", "shift+tab", "shift+tab", "shift+tab") // Stats, Words, Speak, Pairs, Listen, Quiz
	if a.Screen() != Quiz {
		t.Fatalf("screen %v", a.Screen())
	}
	mustContain(t, pageText(a), "Question 4 of 10", "› ho")

	// esc on a multiple-choice item goes Home too.
	a, _ = press(t, a, "backspace", "backspace")
	a = answerRight(t, a)
	a, _ = press(t, a, "enter", "esc")
	if a.Screen() != Home || a.quiz.pos != 4 {
		t.Errorf("screen %v, question %d", a.Screen(), a.quiz.pos+1)
	}
	if len(x.log.deltas) != 0 {
		t.Errorf("an unfinished quiz recorded %v", x.log.deltas)
	}
	x.untouched(t, a)
}

func grammarItems() teacher.GrammarQuiz {
	return teacher.GrammarQuiz{
		Topics: []string{"past simple", "going to"},
		Items: []teacher.QuizItem{
			{Question: "Yesterday I ___ to work.", Options: []string{"go", "went", "goes", "going"}, Answer: 1, Why: "Yesterday pide pasado."},
			{Question: "She ___ TV last night.", Options: []string{"watched", "watch", "watches", "watching"}, Answer: 0, Why: "Pasado regular: -ed."},
			{Question: "I'm ___ visit my mum.", Options: []string{"go", "going", "going to", "went"}, Answer: 2, Why: "Planes: be going to + verbo."},
			{Question: "They ___ at home.", Options: []string{"was", "were", "is", "be"}, Answer: 1, Why: "Con they se usa were."},
		},
	}
}

// startGrammar picks the grammar quiz and delivers the teacher's answer.
func startGrammar(t *testing.T, x quizEnv) App {
	t.Helper()
	a, cmd := press(t, x.App, "down", "enter")
	if cmd == nil {
		t.Fatal("the grammar quiz asked nothing")
	}
	mustContain(t, pageText(a), "Quiz · Grammar", "this takes about 10 seconds")
	return send(t, a, cmd())
}

func TestGrammarQuizRunsTheTeachersItems(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.grammar = grammarItems()
	a := startGrammar(t, x)

	if len(x.ft.grammarCalls) != 1 || x.ft.grammarCalls[0] != nil || x.ft.level != domain.A2 {
		t.Errorf("calls %v, level %v: want one call at A2 without focus", x.ft.grammarCalls, x.ft.level)
	}
	mustContain(t, pageText(a), "Question 1 of 4", "past simple, going to", "Yesterday I ___ to work.", "2  went")

	a, _ = press(t, a, "2")
	mustContain(t, pageText(a), "✓ correct", "Yesterday pide pasado.")
	a, _ = press(t, a, "enter")

	a, _ = press(t, a, "3") // wrong: "watches"
	mustContain(t, pageText(a), "✗", "the answer is watched", "Pasado regular: -ed.")
	a, _ = press(t, a, "enter")
	a, _ = press(t, a, "3", "enter")
	x.clock.t = x.clock.t.Add(25 * time.Minute)
	a, _ = press(t, a, "2")
	mustContain(t, statusBar(a), "enter result")
	a, _ = press(t, a, "enter")

	mustContain(t, pageText(a), "3/4", "She ___ TV last night. → watched", "you chose watches", "Pasado regular: -ed.")
	mustNotContain(t, pageText(a), "Yesterday pide pasado.")
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 10, Activities: []string{"Quiz: grammar 3/4"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	x.untouched(t, a)
}

func TestGrammarQuizTargetsTheRecurringErrors(t *testing.T) {
	x := newQuizApp(t, quizDeck(), map[string]int{"plural": 1, "be": 2, "article": 4, "other": 9})
	x.ft.grammar = grammarItems()
	a, _ := press(t, x.App, "down")
	mustContain(t, pageText(a), "articles (a/an/the)", "to be (am/is/are)")
	startGrammar(t, x)
	if want := [][]string{{"article", "be"}}; !reflect.DeepEqual(x.ft.grammarCalls, want) {
		t.Errorf("focus %v, want %v", x.ft.grammarCalls, want)
	}
}

func TestGrammarQuizFailureShowsTheErrorAndEnterRetries(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.grammarErr = errors.New("pi: timeout")
	a := startGrammar(t, x)
	mustContain(t, pageText(a), "Could not get the quiz", "pi: timeout", "enter to try again")
	mustContain(t, statusBar(a), "enter try again")
	if len(x.log.deltas) != 0 {
		t.Errorf("a failed quiz recorded %v", x.log.deltas)
	}

	x.ft.grammarErr, x.ft.grammar = nil, grammarItems()
	a, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("enter did not ask again")
	}
	a = send(t, a, cmd())
	mustContain(t, pageText(a), "Question 1 of 4")
	if len(x.ft.grammarCalls) != 2 {
		t.Errorf("%d calls, want 2", len(x.ft.grammarCalls))
	}
}

func TestGrammarQuizIgnoresStaleReplies(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.grammarErr = errors.New("first try failed")
	a := startGrammar(t, x)
	stale := quizGrammarMsg{req: a.quiz.req, quiz: grammarItems()}
	a, cmd := press(t, a, "enter") // retry: a new request id
	a = send(t, a, stale)
	if a.quiz.phase != quizLoading {
		t.Fatalf("phase %v: a stale reply was used", a.quiz.phase)
	}
	x.ft.grammarErr, x.ft.grammar = nil, grammarItems()
	a = send(t, a, cmd())
	if a.quiz.phase != quizAsking || len(a.quiz.questions) != 4 {
		t.Fatalf("phase %v, %d questions: the current reply was dropped", a.quiz.phase, len(a.quiz.questions))
	}
}

func TestQuizPagesFitTheSmallestTerminal(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	items := grammarItems()
	for i := len(items.Items); i < teacher.GrammarItems; i++ {
		it := items.Items[i%4]
		it.Question, it.Why = fmt.Sprintf("%d. %s", i+1, it.Question), fmt.Sprintf("%s (%d)", it.Why, i+1)
		items.Items = append(items.Items, it)
	}
	x.ft.grammar = items
	a := send(t, startGrammar(t, x), tea.WindowSizeMsg{Width: 80, Height: 24})
	mustContain(t, pageText(a), "4  going", "Question 1 of 8")
	t.Logf("question:\n%s", screen(a))
	for range teacher.GrammarItems {
		a, _ = press(t, a, wrongOption(question(a)))
		mustContain(t, pageText(a), "Press enter", question(a).why)
		a, _ = press(t, a, "enter")
	}
	t.Logf("result:\n%s", screen(a))
	last := items.Items[teacher.GrammarItems-1]
	mustContain(t, pageText(a), "0/8", last.Question, last.Why)
	mustContain(t, statusBar(a), "n new quiz")
}
