package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// chapter1 and chapter2 are two chapters of the same story.
func chapter1() teacher.StoryChapter {
	return teacher.StoryChapter{
		Title:   "The lost key",
		Text:    "Ana is a teacher. On Monday morning she cannot find her key. She looks in her bag.",
		Summary: "Ana cannot find her key on Monday.",
		Glossary: []teacher.Gloss{
			{Word: "key", Meaning: "llave", Example: "she cannot find her key", New: true},
			{Word: "bag", Meaning: "bolso", Example: "She looks in her bag."},
		},
		Question: teacher.QuizItem{Question: "What can Ana not find?", Options: []string{"Her key", "Her cat", "Her bag"},
			Answer: 0, Why: "Dice: she cannot find her key."},
	}
}

func chapter2() teacher.StoryChapter {
	return teacher.StoryChapter{
		Title:   "The lost key",
		Text:    "Her cat Tom sleeps on the sofa. Under Tom there is the key!",
		Summary: "Ana cannot find her key. Her cat Tom sleeps on it.",
		Glossary: []teacher.Gloss{
			{Word: "sofa", Meaning: "sillón", Example: "Her cat Tom sleeps on the sofa.", New: true},
		},
		Question: teacher.QuizItem{Question: "Where is the key?", Options: []string{"In the bag", "Under the cat", "At school"},
			Answer: 1, Why: "Dice: under Tom there is the key."},
	}
}

// storyShelf is a fake SaveStoryFunc: it remembers every saved story
// and gives new stories the id 7.
type storyShelf struct {
	saved []domain.Story
	err   error
}

func (s *storyShelf) save(st domain.Story) (int64, error) {
	s.saved = append(s.saved, st)
	if st.ID == 0 {
		return 7, s.err
	}
	return st.ID, s.err
}

func (s *storyShelf) last() domain.Story { return s.saved[len(s.saved)-1] }

type storyEnv struct {
	App
	ft    *fakeTeacher
	ad    *adder
	log   *studyLog
	shelf *storyShelf
	clock *testClock
}

// newStoryApp opens Learn with a fake teacher that writes chapters, a
// saved story (zero for none) and the study days days.
func newStoryApp(t *testing.T, saved domain.Story, days []domain.Day, chapters ...teacher.StoryChapter) storyEnv {
	t.Helper()
	x := storyEnv{ft: &fakeTeacher{chapters: chapters}, ad: &adder{}, log: &studyLog{}, shelf: &storyShelf{},
		clock: &testClock{t: fixedNow}}
	a := NewApp(Config{
		Cards: deck(), Save: (&recorder{}).save, Now: x.clock.now, Start: Learn, MaxNew: DefaultMaxNew,
		Profile: domain.Profile{Level: domain.A1}, Record: x.log.record, Days: days,
		Teacher: x.ft, Add: x.ad.add,
		Story: saved, SaveStory: x.shelf.save,
	})
	x.App = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

func (x *storyEnv) press(t *testing.T, keys ...string) tea.Cmd {
	t.Helper()
	var cmd tea.Cmd
	x.App, cmd = press(t, x.App, keys...)
	return cmd
}

func TestStoryFlowChapterQuestionAndTheNextChapter(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1(), chapter2())
	mustContain(t, pageText(x.App), "s a story in chapters")

	// s asks for chapter 1 of a new story at the learner's level.
	cmd := x.press(t, "s")
	mustContain(t, pageText(x.App), "Writing chapter 1 for you")
	x.App = run(t, x.App, cmd)
	if len(x.ft.storyCalls) != 1 || x.ft.storyCalls[0] != (teacher.StorySoFar{}) || x.ft.level != domain.A1 || len(x.ft.known) == 0 {
		t.Fatalf("calls %+v at %s with known %v; want a new A1 story", x.ft.storyCalls, x.ft.level, x.ft.known)
	}
	mustContain(t, pageText(x.App), "The lost key", "Chapter 1 of 5", "Words read this week: 0", "she cannot find her key")

	// enter goes from the text to the glossary; the new word starts marked.
	x.press(t, "enter")
	mustContain(t, pageText(x.App), "Glossary", "[x] key", "llave", "[ ] bag")

	// a adds the marked words and asks the question.
	x.clock.t = x.clock.t.Add(6 * time.Minute)
	x.press(t, "a")
	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 1 || x.ad.added[0][0].Word != "key" {
		t.Fatalf("added = %+v", x.ad.added)
	}
	mustContain(t, pageText(x.App), "What can Ana not find?", "1  Her key", "2  Her cat", "3  Her bag")
	if len(x.log.deltas) != 0 {
		t.Fatalf("recorded before the answer: %+v", x.log.deltas)
	}

	// A wrong answer shows the right one; the chapter is recorded.
	x.press(t, "2")
	mustContain(t, pageText(x.App), "✗ Not quite. The answer is 1: Her key", "Dice: she cannot find her key.")
	words := progress.CountWords(chapter1().Text)
	if len(x.log.deltas) != 1 {
		t.Fatalf("deltas = %+v", x.log.deltas)
	}
	d := x.log.deltas[0]
	if d.Minutes != 6 || d.WordsRead != words || len(d.Activities) != 1 || d.Activities[0] != "Reading: The lost key ch 1" {
		t.Errorf("delta = %+v, want 6 min, %d words, \"Reading: The lost key ch 1\"", d, words)
	}
	mustContain(t, pageText(x.App), fmt.Sprintf("Words read this week: %d", words))
	x.press(t, "3") // a second answer changes nothing
	if len(x.log.deltas) != 1 {
		t.Errorf("answered twice: %+v", x.log.deltas)
	}

	// n continues the same story from its title and summary.
	cmd = x.press(t, "n")
	mustContain(t, pageText(x.App), "Writing chapter 2 for you")
	x.App = run(t, x.App, cmd)
	want := teacher.StorySoFar{Title: "The lost key", Chapter: 1, Summary: "Ana cannot find her key on Monday."}
	if len(x.ft.storyCalls) != 2 || x.ft.storyCalls[1] != want {
		t.Fatalf("second call = %+v, want %+v", x.ft.storyCalls, want)
	}
	mustContain(t, pageText(x.App), "Chapter 2 of 5", "Under Tom there is the key!")

	// A right answer says so; chapter 2 is recorded on its own.
	x.press(t, "enter", "a", "2")
	mustContain(t, pageText(x.App), "✓ Right: Under the cat")
	if len(x.log.deltas) != 2 || x.log.deltas[1].Activities[0] != "Reading: The lost key ch 2" ||
		x.log.deltas[1].WordsRead != progress.CountWords(chapter2().Text) {
		t.Errorf("deltas = %+v", x.log.deltas)
	}
	mustContain(t, pageText(x.App), fmt.Sprintf("Words read this week: %d", words+progress.CountWords(chapter2().Text)))
}

func TestStoryIsSavedAtEachStep(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	x.App = run(t, x.App, x.press(t, "s"))

	if len(x.shelf.saved) != 1 {
		t.Fatalf("saved = %+v", x.shelf.saved)
	}
	st := x.shelf.last()
	if st.ID != 0 || st.Title != "The lost key" || st.Chapter != 1 || st.Summary != chapter1().Summary ||
		st.Text != chapter1().Text || st.Done || !strings.Contains(st.Details, "What can Ana not find?") {
		t.Errorf("first save = %+v", st)
	}

	x.press(t, "enter", "a", "1")
	if len(x.shelf.saved) != 2 || !x.shelf.last().Done || x.shelf.last().ID != 7 {
		t.Errorf("after the answer: %+v", x.shelf.saved)
	}
}

func TestStorySurvivesANewApp(t *testing.T) {
	// The first app gets chapter 1; the learner quits before answering.
	first := newStoryApp(t, domain.Story{}, nil, chapter1())
	first.App = run(t, first.App, first.press(t, "s"))
	saved := first.shelf.last()
	saved.ID = 7

	// A new app opens the same chapter without asking the teacher.
	x := newStoryApp(t, saved, nil)
	if cmd := x.press(t, "s"); cmd != nil {
		t.Error("a saved chapter must not ask the teacher")
	}
	if len(x.ft.storyCalls) != 0 {
		t.Fatalf("calls = %+v", x.ft.storyCalls)
	}
	mustContain(t, pageText(x.App), "The lost key", "Chapter 1 of 5", "she cannot find her key")
	x.press(t, "enter")
	mustContain(t, pageText(x.App), "[x] key")
	x.press(t, "a", "1")
	mustContain(t, pageText(x.App), "✓ Right: Her key")
	if len(x.log.deltas) != 1 || x.log.deltas[0].Activities[0] != "Reading: The lost key ch 1" {
		t.Errorf("deltas = %+v", x.log.deltas)
	}

	// Once answered, a new app goes on with chapter 2 of the same story.
	done := x.shelf.last()
	y := newStoryApp(t, done, nil, chapter2())
	y.App = run(t, y.App, y.press(t, "s"))
	want := teacher.StorySoFar{Title: "The lost key", Chapter: 1, Summary: chapter1().Summary}
	if len(y.ft.storyCalls) != 1 || y.ft.storyCalls[0] != want {
		t.Errorf("calls = %+v, want %+v", y.ft.storyCalls, want)
	}
	if st := y.shelf.last(); st.ID != 7 || st.Chapter != 2 || st.Done {
		t.Errorf("chapter 2 saved as %+v, want the same row, chapter 2", st)
	}
}

func TestStoryAfterTheLastChapterStartsANewOne(t *testing.T) {
	last := domain.Story{ID: 7, Title: "The lost key", Chapter: teacher.StoryChapters, Summary: "The end.", Text: "x", Done: true}
	next := chapter1()
	next.Title = "A day at the sea"
	x := newStoryApp(t, last, nil, next)
	x.App = run(t, x.App, x.press(t, "s"))
	if x.ft.storyCalls[0] != (teacher.StorySoFar{}) {
		t.Errorf("call = %+v, want a new story", x.ft.storyCalls[0])
	}
	if st := x.shelf.last(); st.ID != 0 || st.Chapter != 1 || st.Title != "A day at the sea" {
		t.Errorf("saved %+v, want a new row for chapter 1", st)
	}
	x.press(t, "enter", "a", "1")
	mustContain(t, pageText(x.App), "Press n for the next chapter.")
	mustContain(t, statusBar(x.App), "n next chapter")
}

func TestStoryLastChapterSaysTheEnd(t *testing.T) {
	saved := domain.Story{ID: 7, Title: "The lost key", Chapter: teacher.StoryChapters - 1, Summary: "s", Text: "x", Done: true}
	x := newStoryApp(t, saved, nil, chapter2())
	x.App = run(t, x.App, x.press(t, "s"))
	x.press(t, "enter", "a", "2")
	mustContain(t, pageText(x.App), "The end of the story", "Press n for a new story.")
	mustContain(t, statusBar(x.App), "n new story")
}

func TestStoryWhenTheAIFails(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	x.ft.storyErr = errors.New("the model timed out")

	x.App = run(t, x.App, x.press(t, "s"))

	mustContain(t, pageText(x.App), "✗ Could not get the chapter: the model timed out")
	mustContain(t, statusBar(x.App), "enter try again")
	if len(x.shelf.saved) != 0 || len(x.log.deltas) != 0 {
		t.Errorf("saved %+v, recorded %+v", x.shelf.saved, x.log.deltas)
	}
	x.ft.storyErr = nil
	x.App = run(t, x.App, x.press(t, "enter"))
	mustContain(t, pageText(x.App), "Chapter 1 of 5")
}

func TestStoryWithoutPi(t *testing.T) {
	a := NewApp(Config{Cards: deck(), Save: (&recorder{}).save, Now: (&testClock{t: fixedNow}).now, Start: Learn})
	a = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	a, cmd := press(t, a, "s")
	if cmd != nil {
		t.Error("s must do nothing without a teacher")
	}
	mustContain(t, pageText(a), "The AI teacher is off. Install Gentle Shell (the pi command)")
}

func TestStorySaveErrorIsShown(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	x.shelf.err = errors.New("disk full")
	x.App = run(t, x.App, x.press(t, "s"))
	mustContain(t, pageText(x.App), "✗ Could not save the story: disk full", "Chapter 1 of 5")
}

func TestStoryDamagedSavedChapterGoesOn(t *testing.T) {
	saved := domain.Story{ID: 7, Title: "The lost key", Chapter: 1, Summary: "s", Text: "x", Details: "{not json"}
	x := newStoryApp(t, saved, nil, chapter2())
	x.press(t, "s")
	mustContain(t, pageText(x.App), "Could not read the saved chapter")
	x.App = run(t, x.App, x.press(t, "enter"))
	if x.ft.storyCalls[0].Chapter != 1 {
		t.Errorf("call = %+v, want the chapter after 1", x.ft.storyCalls[0])
	}
}

func TestStorySToggleKeepsTheChapter(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	x.App = run(t, x.App, x.press(t, "s"))
	x.press(t, "s")
	mustContain(t, pageText(x.App), "Read a short text written for you")
	if cmd := x.press(t, "s"); cmd != nil {
		t.Error("coming back must not ask again")
	}
	mustContain(t, pageText(x.App), "Chapter 1 of 5")
}

func TestStoryLongChapterScrolls(t *testing.T) {
	long := chapter1()
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "Line %02d of a long chapter about Ana and her key.\n", i)
	}
	long.Text = b.String()
	x := newStoryApp(t, domain.Story{}, nil, long)
	x.App = send(t, x.App, tea.WindowSizeMsg{Width: 80, Height: 24})
	x.App = run(t, x.App, x.press(t, "s"))
	mustContain(t, pageText(x.App), "Line 00", "↓ more")
	mustNotContain(t, pageText(x.App), "Line 39")
	keys := make([]string, 40)
	for i := range keys {
		keys[i] = "down"
	}
	x.press(t, keys...)
	mustContain(t, pageText(x.App), "Line 39")
	mustNotContain(t, pageText(x.App), "↓ more")
	if n := len(screenLines(x.App)); n != 24 {
		t.Errorf("view has %d lines, want 24", n)
	}
}

func TestStoryReadAloud(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	sp := &fakeSpeaker{}
	x.App.speaker = sp
	x.App = run(t, x.App, x.press(t, "s"))
	mustContain(t, statusBar(x.App), "P read all")
	cmd := x.press(t, "P")
	run(t, x.App, cmd)
	x.press(t, "enter")
	cmd = x.press(t, "p")
	run(t, x.App, cmd)
	if got := sp.texts(); len(got) != 2 || got[0] != chapter1().Text || got[1] != "key" {
		t.Errorf("said = %q", got)
	}
}

func TestHomeShowsWordsReadThisWeek(t *testing.T) {
	// fixedNow is Monday 9 November: Sunday the 8th is last week.
	days := []domain.Day{
		{Date: domain.DateOf(fixedNow).AddDays(-1), WordsRead: 900, Activities: []string{"Reading: x ch 1"}},
		{Date: domain.DateOf(fixedNow), WordsRead: 312, Activities: []string{"Reading: x ch 2"}},
	}
	x := newStoryApp(t, domain.Story{}, days)
	x.App.screen = Home
	mustContain(t, pageText(x.App), "Read    312 words this week")
}

func TestHelpLearnFitsTheSmallestTerminal(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Learn, 80, 24)
	a, _ = press(t, a, "?")
	mustContain(t, pageText(a), "ctrl+c", "a story in chapters", "answer the chapter's question", "p / P")
}

func TestStoryFitsTheSmallestTerminal(t *testing.T) {
	x := newStoryApp(t, domain.Story{}, nil, chapter1())
	x.App = send(t, x.App, tea.WindowSizeMsg{Width: 80, Height: 24})
	check := func(step string) {
		t.Helper()
		lines := screenLines(x.App)
		if len(lines) != 24 {
			t.Errorf("%s: %d lines, want 24", step, len(lines))
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > 80 {
				t.Errorf("%s: line of %d columns: %q", step, w, l)
			}
		}
		t.Logf("%s:\n%s", step, screen(x.App))
	}
	check("idle")
	x.App = run(t, x.App, x.press(t, "s"))
	check("reading")
	x.press(t, "enter")
	check("glossary")
	x.press(t, "a")
	check("question")
	x.press(t, "2")
	check("answered")
	x.press(t, "?")
	check("help")
}
