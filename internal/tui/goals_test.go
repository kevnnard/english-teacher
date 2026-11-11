package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// goalTicker is a fake TickGoalFunc that remembers every tick.
type goalTicker struct {
	ticks []string
	dates []domain.Date
	err   error
}

func (g *goalTicker) tick(id string, d domain.Date) error {
	g.ticks = append(g.ticks, id)
	g.dates = append(g.dates, d)
	return g.err
}

// fixedNow is Monday 9 November 2026: this week is that one day, and
// last week runs from Monday 2 to Sunday 8.
var lastWed = today.AddDays(-5)

func weekDeck() []domain.Card {
	cards := deck()
	look := word("look for", "", "buscar", "")
	look.Added = today
	prod := look
	prod.Direction = domain.Production
	house := reviewed(word("house", "", "casa", ""), day(30), 30)
	house.Added, house.KnownSince = lastWed, today
	return append(cards, look, prod, house)
}

func newGoalsApp(t *testing.T, cfg Config) App {
	t.Helper()
	cfg.Save, cfg.Now, cfg.MaxNew = (&recorder{}).save, func() time.Time { return fixedNow }, DefaultMaxNew
	if cfg.Profile.Level == "" {
		cfg.Profile = domain.Profile{Level: domain.A1, DailyGoalMinutes: 20}
	}
	return send(t, NewApp(cfg), tea.WindowSizeMsg{Width: 120, Height: 30})
}

func TestProgressShowsTheWeekAgainstLastWeek(t *testing.T) {
	a := newGoalsApp(t, Config{
		Cards: weekDeck(),
		Days: []domain.Day{
			{Date: today, Minutes: 15, Reviews: 6, WordsRead: 250,
				Activities: []string{"Writing: 30/40", "Writing: 35/40", "Talk: Introduce yourself"}},
			{Date: lastWed, Minutes: 40, Reviews: 2, WordsRead: 600, Activities: []string{"Writing: 20/40"}},
		},
		Mistakes: []domain.Mistake{
			{Date: today, Kind: "article"}, {Date: lastWed, Kind: "article"}, {Date: lastWed, Kind: "article"},
			{Date: lastWed, Kind: "article"}, {Date: today, Kind: "be"}, {Date: today, Kind: "be"},
		},
	})

	a, _ = press(t, a, "g")
	if a.Screen() != Progress {
		t.Fatalf("g opened %v, want Progress", a.Screen())
	}
	page := pageText(a)
	mustContain(t, page, "this week", "last week",
		"Study time", "15 min", "40 min",
		"New words", "Words learned", "Words read", "250", "600",
		"Writing average", "32.5/40", "20/40",
		"Top errors", "article", "falling", "be", "rising",
		"Can do · A1", "2 of 6 ticked", "✓ I can introduce myself", "✓ I can write short", "○ I can order a drink")
	mustNotContain(t, page, "streak", progress.NoWeekData)
	mustContain(t, statusBar(a), "esc home")
}

func TestProgressWithNothingRecordedSaysSo(t *testing.T) {
	a := newGoalsApp(t, Config{Cards: deck()})
	a, _ = press(t, a, "g")
	page := pageText(a)
	mustContain(t, page, progress.NoWeekData, "Can do · A1", "0 of 6 ticked")
	mustNotContain(t, page, "Study time", "min")
}

func TestProgressGoalCursorShowsHowToProveIt(t *testing.T) {
	days := []domain.Day{{Date: today, Activities: []string{"Talk: Introduce yourself"}}}
	a := newGoalsApp(t, Config{Cards: deck(), Days: days})
	a, _ = press(t, a, "g")
	mustContain(t, pageText(a), "▸ ✓ I can introduce myself", "Proved on 9 Nov")

	a, _ = press(t, a, "down")
	mustContain(t, pageText(a), "▸ ○ I can order a drink", "To tick it: finish the Talk role-play “Order a coffee”")

	for range 10 { // the cursor stops on the last goal, which no activity proves
		a, _ = press(t, a, "j")
	}
	mustContain(t, pageText(a), "▸ – I can fill in a simple form", "No activity in the app proves this one yet.")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Errorf("esc: %v, want Home", a.Screen())
	}
}

func TestHomeShowsTheCanDoGoals(t *testing.T) {
	days := []domain.Day{{Date: today, Activities: []string{"Talk: Introduce yourself"}}}
	a := newGoalsApp(t, Config{Cards: deck(), Days: days})
	mustContain(t, pageText(a), "Goals", "1 of 6 A1 can-do goals")
}

func TestGoalsCountOnlyEvidenceSinceTheLevelStarted(t *testing.T) {
	days := []domain.Day{{Date: lastWed, Activities: []string{"Talk: Introduce yourself"}}}
	a := newGoalsApp(t, Config{Cards: deck(), Days: days,
		Profile: domain.Profile{Level: domain.A1, Started: today, DailyGoalMinutes: 20}})
	mustContain(t, pageText(a), "0 of 6 A1 can-do goals")
}

func TestFinishingATalkTicksItsGoalAndItsErrorsShow(t *testing.T) {
	x := talkApp(t)
	x.ft.turns[0].Done = true
	x.App.profile.Level = domain.A1
	a := chatting(t, x) // "Order a coffee"
	a, wait := say(t, a, coffeeLine)
	a = wait()

	a, _ = press(t, a, "esc", "g")
	mustContain(t, pageText(a), "✓ I can order a drink", "1 of 6 ticked", "word-order", "rising")
}

// storyGoalsApp is Learn with a story chapter and a goal ticker.
func storyGoalsApp(t *testing.T, ticker *goalTicker) App {
	t.Helper()
	a := newGoalsApp(t, Config{Cards: deck(), Start: Learn, Teacher: &fakeTeacher{chapters: []teacher.StoryChapter{chapter1()}},
		Add: (&adder{}).add, SaveStory: (&storyShelf{}).save, TickGoal: ticker.tick, Record: (&studyLog{}).record})
	a, cmd := press(t, a, "s")
	a = run(t, a, cmd)
	a, _ = press(t, a, "enter", "a")
	return a
}

func TestARightStoryAnswerTicksTheReadingGoal(t *testing.T) {
	ticker := &goalTicker{}
	a := storyGoalsApp(t, ticker)
	a, _ = press(t, a, "1")
	if len(ticker.ticks) != 1 || ticker.ticks[0] != "a1-read" || ticker.dates[0] != today {
		t.Fatalf("ticks = %v on %v, want a1-read today", ticker.ticks, ticker.dates)
	}
	a, _ = press(t, a, "esc", "g")
	mustContain(t, pageText(a), "✓ I can read a short, simple text")
}

func TestAWrongStoryAnswerTicksNothing(t *testing.T) {
	ticker := &goalTicker{}
	a := storyGoalsApp(t, ticker)
	a, _ = press(t, a, "2")
	if len(ticker.ticks) != 0 {
		t.Errorf("ticks = %v, want none", ticker.ticks)
	}
	a, _ = press(t, a, "esc", "g")
	mustContain(t, pageText(a), "○ I can read a short, simple text")
}

func TestAStoredTickIsShownAndAFailedTickIsNot(t *testing.T) {
	a := newGoalsApp(t, Config{Cards: deck(), GoalTicks: map[string]domain.Date{"a1-read": lastWed}})
	a, _ = press(t, a, "g")
	mustContain(t, pageText(a), "✓ I can read a short, simple text")

	ticker := &goalTicker{err: errors.New("disk full")}
	b := storyGoalsApp(t, ticker)
	b, _ = press(t, b, "1")
	mustContain(t, pageText(b), "disk full")
	b, _ = press(t, b, "esc", "g")
	mustContain(t, pageText(b), "○ I can read a short, simple text")
}

func TestReviewKeepsTheDayAWordBecameKnown(t *testing.T) {
	rec := &recorder{}
	almost := reviewed(word("hello", "", "Hola", ""), time.Time{}, 20)
	known := reviewed(word("hi", "", "Hola", ""), time.Time{}, 30)
	known.KnownSince = lastWed
	almost.Repetitions, known.Repetitions = 3, 3 // past SM-2's first steps: the interval grows by the ease
	a := newApp(t, []domain.Card{almost, known}, rec, Review, 120, 30)

	a, _ = press(t, a, "space", "3", "space", "3")
	if len(rec.saved) != 2 {
		t.Fatalf("saved %d cards, want 2", len(rec.saved))
	}
	for _, c := range rec.saved {
		want := map[string]domain.Date{"hello": today, "hi": lastWed}[c.Word]
		if c.KnownSince != want || c.IntervalDays < domain.KnownIntervalDays {
			t.Errorf("%s: KnownSince = %v, want %v", c.Word, c.KnownSince, want)
		}
	}
}

func TestNewCardsAreAddedWithTodaysDate(t *testing.T) {
	x := loadedLearn(t)
	a, _ := press(t, x.App, "a")
	if len(x.ad.added) != 1 || len(x.ad.added[0]) == 0 {
		t.Fatalf("added = %+v", x.ad.added)
	}
	for _, c := range x.ad.added[0] {
		if c.Added != today {
			t.Errorf("%s: Added = %v, want %v", c.Word, c.Added, today)
		}
	}
	for _, c := range a.cards {
		if c.Word == x.ad.added[0][0].Word && c.Added != today {
			t.Errorf("deck %s (%v): Added = %v, want %v", c.Word, c.Direction, c.Added, today)
		}
	}
}

func TestProgressFitsOn80x24WithEveryRow(t *testing.T) {
	a := newGoalsApp(t, Config{
		Cards:   weekDeck(),
		Profile: domain.Profile{Level: domain.A2, DailyGoalMinutes: 20},
		Days:    []domain.Day{{Date: today, Minutes: 5, Activities: []string{"Writing: 30/40"}}},
		Mistakes: []domain.Mistake{
			{Date: today, Kind: "article"}, {Date: today, Kind: "be"}, {Date: lastWed, Kind: "tense"}, {Date: lastWed, Kind: "tense"}, {Date: today, Kind: "spelling"},
		},
	})
	a = send(t, a, tea.WindowSizeMsg{Width: 80, Height: 24})
	a, _ = press(t, a, "g")
	for range 6 {
		a, _ = press(t, a, "down")
	}
	page := pageText(a)
	mustContain(t, page, "Week", "Writing average", "Top errors", "tense", "falling", "Can do · A2",
		"▸ ○ I can tell a simple story", "To tick it: finish a 4/3/2 session in Fluency")
	for _, l := range screenLines(a) {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line is %d columns wide: %q", w, l)
		}
	}
}
