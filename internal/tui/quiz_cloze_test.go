package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// lookingFor and couldYou are the chunks of the cloze items below.
var (
	lookingFor = domain.Chunk{Phrase: "I'm looking for…", Meaning: "Estoy buscando…", Use: "en una tienda", Example: "I'm looking for a jacket."}
	couldYou   = domain.Chunk{Phrase: "Could you…?", Meaning: "¿Podrías…?", Use: "para pedir con cortesía", Example: "Could you help me?"}
)

// clozeItems is a round of three items on three points, as the teacher
// returns it: already interleaved.
func clozeItems() teacher.ClozeQuiz {
	return teacher.ClozeQuiz{
		Points: []string{"past simple", "comparatives", "going to"},
		Items: []domain.ClozeItem{
			{Sentence: "Tom went home yesterday.", Answer: "went", Hint: "go", Point: "past simple",
				Rule: "Pasado simple: acción terminada.", Chunk: lookingFor, Yours: true},
			{Sentence: "This jacket is cheaper than that one.", Answer: "cheaper", Hint: "cheap", Point: "comparatives",
				Rule: "Adjetivo corto + -er."},
			{Sentence: "Could you tell me what you are going to do?", Answer: "going to", Point: "going to",
				Rule: "Planes: be going to + verbo.", Chunk: couldYou},
		},
	}
}

// startCloze picks the cloze round, the third quiz, and delivers the
// teacher's answer.
func startCloze(t *testing.T, x quizEnv) App {
	t.Helper()
	a, cmd := press(t, x.App, "down", "down", "enter")
	if cmd == nil {
		t.Fatal("the cloze round asked nothing")
	}
	mustContain(t, pageText(a), "Quiz · Grammar in context", "this takes about 10 seconds")
	return send(t, a, cmd())
}

// bodyText is the page without the frame on the left and the padding on
// the right, so tests can match whole lines exactly.
func bodyText(a App) string {
	lines := strings.Split(pageText(a), "\n")
	for i, l := range lines {
		if j := strings.Index(l, "│"); j >= 0 {
			l = strings.TrimPrefix(l[j+len("│"):], "   ")
		}
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// answerCloze types s as the answer of the gap on screen.
func answerCloze(t *testing.T, a App, s string) App {
	t.Helper()
	a = typeText(t, a, s)
	a, _ = press(t, a, "enter")
	return a
}

func TestClozeRoundAsksWithTheLearnersSentencesAndMistakes(t *testing.T) {
	x := newQuizApp(t, quizDeck(), map[string]int{"article": 4, "be": 2, "spelling": 9})
	x.App.story.saved = domain.Story{Title: "The lost key", Chapter: 1, Text: "Ana lost her key. She looked everywhere!"}
	x.ft.cloze = clozeItems()

	a, _ := press(t, x.App, "down", "down")
	mustContain(t, pageText(a), "▸ Grammar in context", "gaps in your sentences", "mixing: articles (a/an/the), to be (am/is/are), past")
	startCloze(t, x)

	if len(x.ft.clozeCalls) != 1 || x.ft.level != domain.A2 {
		t.Fatalf("calls %v, level %v: want one call at A2", x.ft.clozeCalls, x.ft.level)
	}
	got := x.ft.clozeCalls[0]
	if want := []string{"spelling", "article", "be"}; !reflect.DeepEqual(got.mistakes, want) {
		t.Errorf("mistakes %v, want %v (the recurring Write mistakes, most first)", got.mistakes, want)
	}
	// The story first, then the card examples, newest first, each once.
	want := []string{"Ana lost her key.", "She looked everywhere!", "I say water every day.", "I say coffee every day."}
	if len(got.sentences) < len(want) || !reflect.DeepEqual(got.sentences[:len(want)], want) {
		t.Errorf("sentences %q, want them to start with %q", got.sentences, want)
	}
	if len(got.sentences) != teacher.MaxOwnSentences {
		t.Errorf("%d sentences, want the cap %d", len(got.sentences), teacher.MaxOwnSentences)
	}
}

func TestClozeRoundShowsTheGapAndTheHintButNotThePoint(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)

	page := bodyText(a)
	mustContain(t, page, "Quiz · Grammar in context\nGap 1 of 3 · ✓ 0\n\n"+
		"Type the missing words:\n  Tom ____ home yesterday.  (go)\n  from your own sentences\n\n"+
		"Your answer\n  › ▏")
	mustNotContain(t, page, "past simple", "Pasado simple", "I'm looking for")
	mustContain(t, statusBar(a), "type the missing words", "enter check")

	// Typing takes every key as a letter, even a, n and q.
	a = typeText(t, a, "an q")
	mustContain(t, pageText(a), "› an q▏")
}

func TestClozeRightAnswerForgivesCaseAndSpacesAndShowsThePoint(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)

	a = answerCloze(t, a, "WENT  ")

	page := bodyText(a)
	mustContain(t, page, "› WENT\n  ✓ correct\n  past simple\n  Chunk: I'm looking for… = Estoy buscando…")
	mustNotContain(t, page, "Pasado simple") // the rule is for a wrong answer
	mustContain(t, statusBar(a), "a add the chunk", "enter next")
}

func TestClozeWrongAnswerShowsTheAnswerAndTheRuleOnOneLine(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)
	a = answerCloze(t, a, "went")
	a, _ = press(t, a, "enter")

	a = answerCloze(t, a, "more cheap")

	mustContain(t, bodyText(a), "  This jacket is _______ than that one.  (cheap)\n",
		"› more cheap\n  ✗ not quite · the answer is cheaper\n  comparatives · Adjetivo corto + -er.\n\nPress enter for the next question.")
	mustContain(t, statusBar(a), "enter next")
	mustNotContain(t, statusBar(a), "a add") // this item has no chunk
}

func TestClozeChunkIsAddedOnceAndShowsInReview(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)
	a = answerCloze(t, a, "goed") // wrong; the chunk is shown

	a, _ = press(t, a, "a")
	mustContain(t, pageText(a), "✓ added to Review")
	a, _ = press(t, a, "a")
	mustContain(t, pageText(a), "already in your cards")
	if len(x.ad.added) != 1 {
		t.Fatalf("%d adds, want 1: %v", len(x.ad.added), x.ad.added)
	}
	// The chunk card is stored with the day it was added.
	if want := dated([]domain.Card{lookingFor.Card()}, today); !reflect.DeepEqual(x.ad.added[0], want) {
		t.Errorf("added %+v, want %+v", x.ad.added[0], want)
	}

	// The last item is missed too: its chunk is added at the end, and the
	// first one is not added again.
	a, _ = press(t, a, "enter")
	a = answerCloze(t, a, "cheaper")
	a, _ = press(t, a, "enter")
	a = answerCloze(t, a, "gonna")
	a, _ = press(t, a, "enter")
	if want := [][]domain.Card{dated([]domain.Card{lookingFor.Card()}, today), dated([]domain.Card{couldYou.Card()}, today)}; !reflect.DeepEqual(x.ad.added, want) {
		t.Errorf("added %+v, want %+v", x.ad.added, want)
	}
	mustContain(t, pageText(a), "1/3", "Added to Review: Could you…?")

	// Both chunks are in the deck in both directions, and Review asks them.
	var chunks []domain.Card
	for _, c := range a.cards {
		if c.Word == lookingFor.Phrase || c.Word == couldYou.Phrase {
			chunks = append(chunks, c)
		}
	}
	if len(chunks) != 4 {
		t.Fatalf("%d chunk cards in the deck, want 4 (2 chunks × 2 directions)", len(chunks))
	}
	r := newApp(t, chunks[:2], &recorder{}, Review, 120, 30)
	mustContain(t, pageText(r), "I'm looking for…")
	r, _ = press(t, r, "space")
	mustContain(t, pageText(r), "Estoy buscando… — en una tienda", "I'm looking for a jacket.")
}

func TestClozeChunkAlreadyInTheDeckIsNotAddedAgain(t *testing.T) {
	cards := append(quizDeck(), lookingFor.Card())
	x := newQuizApp(t, cards, nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)
	a = answerCloze(t, a, "went")

	a, _ = press(t, a, "a")

	mustContain(t, pageText(a), "already in your cards")
	if len(x.ad.added) != 0 {
		t.Errorf("added %v, want nothing", x.ad.added)
	}
}

func TestClozeRoundRecordsAGrammarQuiz(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.cloze = clozeItems()
	a := startCloze(t, x)
	a = answerCloze(t, a, "went")
	a, _ = press(t, a, "enter")
	a = answerCloze(t, a, "cheap")
	a, _ = press(t, a, "enter")
	x.clock.t = x.clock.t.Add(4 * time.Minute)
	a = answerCloze(t, a, "Going to.")
	mustContain(t, statusBar(a), "enter result")
	a, _ = press(t, a, "enter")

	mustContain(t, pageText(a), "2/3", "This jacket is _______ than that one. → cheaper · you wrote cheap",
		"comparatives · Adjetivo corto + -er.")
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 4, Activities: []string{"Quiz: grammar 2/3"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	// Only missed items' chunks are added; this round missed none with one.
	if len(x.ad.added) != 0 {
		t.Errorf("added %v, want nothing", x.ad.added)
	}
	x.untouched(t, a)
	a, _ = press(t, a, "n")
	mustContain(t, pageText(a), "▸ Grammar in context")
}

func TestClozeRoundWithoutTheTeacherSaysSo(t *testing.T) {
	a := newApp(t, quizDeck(), &recorder{}, Quiz, 120, 30)
	a, cmd := press(t, a, "down", "down", "enter")
	if cmd != nil || a.quiz.phase != quizPick {
		t.Errorf("phase %v, cmd %v: the cloze round started without a teacher", a.quiz.phase, cmd != nil)
	}
	mustContain(t, strings.Join(strings.Fields(bodyText(a)), " "), "The AI teacher is off, so there is no grammar in context. "+
		"Install Gentle Shell (the pi command) and see `teacher models`.")
}

func TestClozeRoundFailureShowsTheErrorAndEnterRetries(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.clozeErr = errors.New("teacher: the items cover only 2 grammar points; a round mixes at least 3")
	a := startCloze(t, x)
	mustContain(t, pageText(a), "✗ Could not get the quiz: teacher: the items cover only 2 grammar points",
		"Press enter to try again.")
	mustContain(t, statusBar(a), "enter try again")
	if len(x.log.deltas) != 0 {
		t.Errorf("a failed round recorded %v", x.log.deltas)
	}

	x.ft.clozeErr, x.ft.cloze = nil, clozeItems()
	a, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("enter did not ask again")
	}
	a = send(t, a, cmd())
	mustContain(t, pageText(a), "Gap 1 of 3")
	if len(x.ft.clozeCalls) != 2 || len(x.ft.grammarCalls) != 0 {
		t.Errorf("%d cloze calls and %d grammar calls, want 2 and 0", len(x.ft.clozeCalls), len(x.ft.grammarCalls))
	}
}

func TestClozeIgnoresStaleReplies(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	x.ft.clozeErr = errors.New("first try failed")
	a := startCloze(t, x)
	stale := quizClozeMsg{req: a.quiz.req, quiz: clozeItems()}
	a, _ = press(t, a, "enter") // retry: a new request id
	a = send(t, a, stale)
	if a.quiz.phase != quizLoading {
		t.Fatalf("phase %v: a stale reply was used", a.quiz.phase)
	}
}

func TestClozePagesFitTheSmallestTerminal(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	round := clozeItems()
	for i := len(round.Items); i < teacher.ClozeItems; i++ {
		it := round.Items[i%3]
		it.Sentence, it.Rule = fmt.Sprintf("%s (%d)", it.Sentence, i+1), fmt.Sprintf("%s (%d)", it.Rule, i+1)
		it.Chunk = domain.Chunk{Phrase: fmt.Sprintf("chunk %d", i+1), Meaning: "algo"}
		round.Items = append(round.Items, it)
	}
	x.ft.cloze = round
	a := send(t, startCloze(t, x), tea.WindowSizeMsg{Width: 80, Height: 24})
	for range teacher.ClozeItems {
		a = answerCloze(t, a, "wrong")
		mustContain(t, pageText(a), "Press enter", question(a).why)
		a, _ = press(t, a, "enter")
	}
	t.Logf("result:\n%s", screen(a))
	last := round.Items[teacher.ClozeItems-1]
	mustContain(t, pageText(a), "0/8", "Added to Review: I'm looking for…, Could you…?, chunk 4", last.Rule)
	mustContain(t, statusBar(a), "n new quiz")
	if n := len(screenLines(a)); n > 24 {
		t.Errorf("%d lines, want at most 24", n)
	}
}

func TestQuizHelpFitsTheSmallestTerminal(t *testing.T) {
	a := newApp(t, quizDeck(), &recorder{}, Quiz, 80, 24)
	a, _ = press(t, a, "?")
	quiz := helpSections[0]
	for _, sec := range helpSections {
		if sec.title == "Quiz" {
			quiz = sec
		}
	}
	last := quiz.keys[len(quiz.keys)-1][1]
	mustContain(t, pageText(a), "ctrl+c", "add the chunk", last)
	if strings.Count(screen(a), "\n") >= 24 {
		t.Errorf("help is taller than 24 rows:\n%s", screen(a))
	}
}
