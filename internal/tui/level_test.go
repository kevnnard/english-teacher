package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// profileLog is a fake SaveProfile function that remembers every profile.
type profileLog struct {
	saved []domain.Profile
	err   error
}

func (p *profileLog) save(pr domain.Profile) error {
	p.saved = append(p.saved, pr)
	return p.err
}

// levelEnv is an app wired for the Level screen and the level check.
type levelEnv struct {
	App
	ft  *fakeTeacher
	log *studyLog
	pl  *profileLog
}

var fixedToday = domain.DateOf(fixedNow)

// knownDeck is n words known at fixedNow: an interval of 30 days, not due.
func knownDeck(n int) []domain.Card {
	cards := make([]domain.Card, n)
	for i := range cards {
		cards[i] = reviewed(word(fmt.Sprintf("word%d", i), "", "palabra", ""), day(20), 30)
	}
	return cards
}

// readyDays is a log that meets every score criterion.
func readyDays() []domain.Day {
	acts := []string{
		"Writing: 33/40", "Writing: 34/40", "Writing: 35/40", "Writing: 32/40", "Writing: 36/40",
		"Quiz: grammar 7/8", "Quiz: grammar 8/8", "Quiz: grammar 7/8",
		"Listen: 9/10", "Listen: 10/10", "Listen: 9/10",
		"Pairs: 18/20", "Pairs: 19/20", "Pairs: 18/20",
	}
	days := make([]domain.Day, len(acts))
	for i, act := range acts {
		days[i] = domain.Day{Date: fixedToday.AddDays(i - len(acts)), Activities: []string{act}}
	}
	return days
}

func sampleCheckReading() teacher.CheckReading {
	opts := []string{"right", "wrong", "no", "never"}
	return teacher.CheckReading{
		Title: "The market",
		Text:  "Every Sunday Ana goes to the market with her brother.",
		Questions: []teacher.QuizItem{
			{Question: "When does Ana go?", Options: opts, Answer: 0},
			{Question: "Where does she go?", Options: opts, Answer: 0},
			{Question: "Who goes with her?", Options: opts, Answer: 0},
		},
	}
}

// newLevelEnv builds an A1 learner; ready meets every criterion.
func newLevelEnv(t *testing.T, ready bool, w, h int) levelEnv {
	t.Helper()
	x := levelEnv{
		ft: &fakeTeacher{
			checkReading: sampleCheckReading(),
			task:         teacher.WritingTask{Task: "Write about your weekend."},
			correction:   teacher.Correction{Corrected: "ok", Rubric: teacher.Rubric{Grammar: 8, Vocabulary: 8, Coherence: 7, Task: 7}},
			grammar:      grammarItems(),
		},
		log: &studyLog{}, pl: &profileLog{},
	}
	cfg := Config{
		Cards: deck(), Save: (&recorder{}).save, Now: func() time.Time { return fixedNow }, Start: Home,
		MaxNew: DefaultMaxNew, Record: x.log.record, Teacher: x.ft, SaveProfile: x.pl.save,
		Profile: domain.Profile{Level: domain.A1, DailyGoalMinutes: 20, Started: fixedToday.AddDays(-21)},
	}
	if ready {
		cfg.Cards = knownDeck(500)
		cfg.Days = readyDays()
	}
	x.App = send(t, NewApp(cfg), tea.WindowSizeMsg{Width: w, Height: h})
	return x
}

func TestHomeShowsTheNextLevelLine(t *testing.T) {
	x := newLevelEnv(t, false, 80, 24)
	mustContain(t, pageText(x.App), "Level   A1 → A2 · 1 of 6 ready   l")

	x = newLevelEnv(t, true, 80, 24)
	mustContain(t, pageText(x.App), "Level   A1 · Ready for A2 · take the level check   l")

	x.App.profile.Level = domain.C2
	mustContain(t, pageText(x.App), "Level   C2 · the top level")
}

func TestLevelLineFitsEightyColumns(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	for _, l := range screenLines(x.App) {
		if n := len([]rune(l)); n > 80 {
			t.Errorf("line is %d wide: %q", n, l)
		}
	}
}

func TestLevelScreenListsEachCriterion(t *testing.T) {
	x := newLevelEnv(t, false, 80, 24)
	a, _ := press(t, x.App, "l")
	if a.Screen() != Level {
		t.Fatalf("screen = %v, want Level", a.Screen())
	}
	page := pageText(a)
	mustContain(t, page,
		"Level A1 (Beginner) → A2 (Elementary) · 1 of 6 ready",
		"✗ Known words    1                    ≥ 500",
		"    Review every day; a word is known after 21 days.",
		"✗ Writing        no data yet          avg ≥ 32/40 over 5",
		"    Write a text in Write and fix every marked error.",
		"✗ Grammar quiz   no data yet          ≥ 80% over 3",
		"✗ Listen         no data yet          ≥ 85% over 3",
		"✗ Pairs          no data yet          ≥ 85% over 3",
		"✓ Time at level  21 days              ≥ 21 days",
		"The level check opens when every line has a ✓.",
	)
	// Met criteria get no tip.
	mustNotContain(t, page, "this one only needs time")
	mustContain(t, statusBar(a), "esc home")
}

func TestLevelScreenAtTheTopLevel(t *testing.T) {
	x := newLevelEnv(t, false, 80, 24)
	x.App.profile.Level = domain.C2
	a, cmd := press(t, x.App, "l", "enter")
	mustContain(t, pageText(a), "Level C2 (Proficiency) · the top level")
	if cmd != nil || len(x.ft.checkCalls) != 0 {
		t.Error("C2 offered a level check")
	}
}

func TestLevelCheckIsOfferedOnlyWhenEveryCriterionIsMet(t *testing.T) {
	x := newLevelEnv(t, false, 80, 24)
	a, cmd := press(t, x.App, "l", "enter")
	if cmd != nil || len(x.ft.checkCalls) != 0 || a.check.stage != checkOff {
		t.Fatalf("a level check started before every criterion was met")
	}
	mustNotContain(t, pageText(a), "take the level check")

	x = newLevelEnv(t, true, 80, 24)
	a, _ = press(t, x.App, "l")
	mustContain(t, pageText(a), "Ready for A2.", "enter  take the level check")
	mustContain(t, statusBar(a), "enter level check")
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	if fmt.Sprint(x.ft.checkCalls) != "[A2]" {
		t.Errorf("check reading calls = %v, want [A2]", x.ft.checkCalls)
	}
	mustContain(t, pageText(a), "Level check · A2 · reading", "Every Sunday Ana goes", "Question 1 of 3", "When does Ana go?", "1 right")
}

// passReading answers the three reading questions; right picks option 1.
func passReading(t *testing.T, a App, keys ...string) App {
	t.Helper()
	a, cmd := press(t, a, "l", "enter")
	a = run(t, a, cmd)
	a, _ = press(t, a, keys...)
	return a
}

// doWriting runs the writing part: the task at the next level, the text
// and its correction.
func doWriting(t *testing.T, x levelEnv, a App) App {
	t.Helper()
	a, cmd := press(t, a, "enter")
	if a.Screen() != Write {
		t.Fatalf("screen = %v, want Write", a.Screen())
	}
	a = run(t, a, cmd)
	if x.ft.level != domain.A2 {
		t.Errorf("writing task level = %v, want A2", x.ft.level)
	}
	mustContain(t, pageText(a), "Write · level check A2")
	a = typeText(t, a, "I went to the park.")
	a, cmd = press(t, a, "ctrl+d")
	a = run(t, a, cmd)
	if x.ft.level != domain.A2 {
		t.Errorf("correction level = %v, want A2", x.ft.level)
	}
	return a
}

// doQuiz runs the grammar quiz part, answering with keys.
func doQuiz(t *testing.T, x levelEnv, a App, keys ...string) App {
	t.Helper()
	a, cmd := press(t, a, "enter")
	if a.Screen() != Quiz {
		t.Fatalf("screen = %v, want Quiz", a.Screen())
	}
	a = run(t, a, cmd)
	if x.ft.level != domain.A2 {
		t.Errorf("quiz level = %v, want A2", x.ft.level)
	}
	mustContain(t, pageText(a), "Quiz · level check A2")
	for _, k := range keys {
		a, _ = press(t, a, k, "enter")
	}
	return a
}

func TestPassingTheLevelCheckRaisesTheLevel(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	a := passReading(t, x.App, "1", "1", "2")
	mustContain(t, pageText(a), "Reading: 2/3", "enter  write a short text at A2")

	a = doWriting(t, x, a)
	if a.Screen() != Level {
		t.Fatalf("screen = %v, want Level after the correction", a.Screen())
	}
	mustContain(t, pageText(a), "Writing: 30/40", "enter  take the grammar quiz at A2")

	a = doQuiz(t, x, a, "2", "1", "3", "1") // 3 of 4 right: 75%
	if a.Screen() != Level {
		t.Fatalf("screen = %v, want Level after the quiz", a.Screen())
	}
	mustContain(t, pageText(a), "Passed! You are now A2 (Elementary).",
		"✓ Reading        2/3", "✓ Writing        30/40", "✓ Grammar quiz   3/4")

	if a.profile.Level != domain.A2 || a.profile.Started != fixedToday {
		t.Errorf("profile = %+v, want A2 started today", a.profile)
	}
	if len(x.pl.saved) != 1 || x.pl.saved[0].Level != domain.A2 || x.pl.saved[0].Started != fixedToday {
		t.Errorf("saved profiles = %+v", x.pl.saved)
	}
	mustContain(t, activities(x.log), "Level check: passed A2", "Writing: 30/40", "Quiz: grammar 3/4")
	mustContain(t, statusBar(a), "A2")

	a, _ = press(t, a, "esc")
	mustContain(t, pageText(a), "Level   A2 → B1 ·")
}

func TestFailingTheLevelCheckKeepsTheLevel(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	a := passReading(t, x.App, "2", "2", "1") // 1 of 3
	mustContain(t, pageText(a), "Reading: 1/3")
	a = doWriting(t, x, a)
	a = doQuiz(t, x, a, "1", "2", "1", "3") // 0 of 4

	mustContain(t, pageText(a), "Not yet. You stay at A1 for now.",
		"✗ Reading        1/3                  needs 2/3",
		"✓ Writing        30/40",
		"✗ Grammar quiz   0/4                  needs 70%")
	if a.profile.Level != domain.A1 || a.profile.Started != fixedToday.AddDays(-21) {
		t.Errorf("profile = %+v, want A1 unchanged", a.profile)
	}
	if len(x.pl.saved) != 0 {
		t.Errorf("a failed check saved %+v", x.pl.saved)
	}
	acts := activities(x.log)
	mustContain(t, acts, "Level check: not yet")
	mustNotContain(t, acts, "passed")

	// enter goes back to the criteria. The check's own quiz is logged like
	// any grammar quiz, so a failed one lowers that criterion for now.
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "5 of 6 ready", "✗ Grammar quiz   75% over 3")
}

func TestOnlyTheCheckUsesTheNextLevel(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	a, cmd := press(t, x.App, "tab", "tab", "tab", "enter") // Home → Write; ask a task
	if a.Screen() != Write {
		t.Fatalf("screen = %v", a.Screen())
	}
	run(t, a, cmd)
	if x.ft.level != domain.A1 {
		t.Errorf("a normal task asked level %v, want A1", x.ft.level)
	}
}

func TestStoppingTheLevelCheck(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	a := passReading(t, x.App, "1")
	a, _ = press(t, a, "x")
	if a.check.stage != checkOff || a.studyLevel() != domain.A1 {
		t.Errorf("stage = %v, study level = %v", a.check.stage, a.studyLevel())
	}
	mustContain(t, pageText(a), "enter  take the level check")
	if len(x.log.deltas) != 0 || len(x.pl.saved) != 0 {
		t.Error("a stopped check recorded something")
	}
}

func TestLevelCheckReadingFailureCanBeRetried(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	x.ft.checkErr = fmt.Errorf("pi timed out")
	a, cmd := press(t, x.App, "l", "enter")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "✗ Could not get the reading: pi timed out", "Press enter to try again.")
	x.ft.checkErr = nil
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "Question 1 of 3")
}

func TestLevelCheckSaveFailureKeepsTheLevel(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	x.pl.err = fmt.Errorf("disk full")
	a := passReading(t, x.App, "1", "1", "1")
	a = doWriting(t, x, a)
	a = doQuiz(t, x, a, "2", "1", "3", "2")
	mustContain(t, pageText(a), "✗ Could not save your new level: disk full")
	if a.profile.Level != domain.A1 {
		t.Errorf("level = %v, want A1 after a failed save", a.profile.Level)
	}
	mustNotContain(t, activities(x.log), "Level check: passed")
}

func TestLevelHelpFitsTheSmallestTerminal(t *testing.T) {
	x := newLevelEnv(t, true, 80, 24)
	a, _ := press(t, x.App, "?")
	mustContain(t, pageText(a), "your level and the level check")
	a, _ = press(t, a, "?", "l", "?")
	page := pageText(a)
	mustContain(t, page, "take the level check", "ctrl+c")
}

// activities joins every recorded activity, one per line.
func activities(l *studyLog) string {
	var acts []string
	for _, d := range l.deltas {
		acts = append(acts, d.Activities...)
	}
	return strings.Join(acts, "\n")
}
