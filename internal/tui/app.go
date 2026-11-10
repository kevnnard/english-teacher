// Package tui holds the Bubbletea app of the teacher: one root model that
// owns the screens, the notebook page layout and the global keys.
package tui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

// SaveFunc persists a card after the learner grades it.
type SaveFunc func(domain.Card) error

// RecordFunc adds a delta to the learner's study day, like
// store.RecordStudy: counts are summed, and a negative count takes back
// an undone grade.
type RecordFunc func(domain.Day) error

// Screen identifies one page of the app.
type Screen int

const (
	Home Screen = iota
	Review
	Learn
	Write
	Words
	Stats
)

// screens is the order of the tabs and of tab / shift+tab.
var screens = []Screen{Home, Review, Learn, Write, Words, Stats}

func (s Screen) String() string {
	return [...]string{"Home", "Review", "Learn", "Write", "Words", "Stats"}[s]
}

// Config is what the app needs from the outside world.
type Config struct {
	Cards  []domain.Card    // every card in the store, in both directions
	Save   SaveFunc         // called after each grade and undo
	Now    func() time.Time // injected clock
	Start  Screen           // the screen to open on
	MaxNew int              // most never-reviewed cards per session

	Profile domain.Profile // the zero profile means a new A1 learner
	Days    []domain.Day   // recorded study days, for the streak and the week
	Record  RecordFunc     // called after each grade and undo; nil records nothing

	Teacher Teacher // the AI teacher; nil turns Learn and Write off
	Add     AddFunc // stores cards the teacher made; nil adds nothing
}

// DefaultMaxNew is the usual cap on new cards in one session.
const DefaultMaxNew = 20

// App is the root Bubbletea model.
type App struct {
	cards []domain.Card
	save  SaveFunc
	now   func() time.Time

	profile   domain.Profile
	days      []domain.Day
	record    RecordFunc
	lastStudy time.Time     // when the learner last graded a card
	studied   time.Duration // study time not yet recorded as a whole minute

	teacher Teacher
	add     AddFunc
	learn   learnState

	screen        Screen
	width, height int // terminal size; zero until the first WindowSizeMsg
	st            styles
	notes         bool // the learner wants the margin notes under the review card
	help          bool
	palette       palette
	review        session // today's session; its counts feed the status bar
	round         session // the mistakes round, which never saves
	inRound       bool    // the mistakes round is on screen
	words         wordList
	err           error // a failed save stops the app
}

// NewApp returns the app on cfg.Start.
func NewApp(cfg Config) App {
	cards := slices.Clone(cfg.Cards)
	queue := srs.PlanSession(cards, cfg.Now(), cfg.MaxNew)
	profile := cfg.Profile
	if profile.Level == "" {
		profile.Level = domain.A1
	}
	if profile.DailyGoalMinutes <= 0 {
		profile.DailyGoalMinutes = domain.DefaultDailyGoalMinutes
	}
	return App{
		cards:     cards,
		save:      cfg.Save,
		now:       cfg.Now,
		profile:   profile,
		days:      slices.Clone(cfg.Days),
		record:    cfg.Record,
		teacher:   cfg.Teacher,
		add:       cfg.Add,
		lastStudy: cfg.Now(),
		screen:    cfg.Start,
		st:        newStyles(true), // until the terminal reports its background
		notes:     true,
		review:    newSession(cards, queue, false, cfg.Now()),
	}
}

// Init asks the terminal for its background color so the palette can follow it.
func (a App) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Screen returns the screen on display.
func (a App) Screen() Screen { return a.screen }

// Err returns the save error that ended the app, if any.
func (a App) Err() error { return a.err }

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		a.st = newStyles(msg.IsDark())
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	case readingMsg:
		return a.gotReading(msg), nil
	}
	return a, nil
}

// handleKey routes a key press: overlays and text inputs first, then the
// global keys, then the active screen.
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == "ctrl+c":
		return a, tea.Quit
	case a.err != nil:
		return a, tea.Quit // any key leaves the error screen
	case a.palette.open:
		return a.updatePalette(msg)
	case a.help:
		if k == "?" || k == "esc" {
			a.help = false
		}
		return a, nil
	case a.typing():
		return a.updateScreen(msg) // letters belong to the text input
	}

	switch k {
	case "tab":
		a.screen = screens[(int(a.screen)+1)%len(screens)]
	case "shift+tab":
		a.screen = screens[(int(a.screen)+len(screens)-1)%len(screens)]
	case ":":
		a.palette = palette{open: true}
	case "?":
		a.help = true
	case "esc":
		a.screen = Home
	case "c":
		a.notes = !a.notes
	case "q":
		if a.screen == Home {
			return a, tea.Quit
		}
		a.screen = Home
	default:
		return a.updateScreen(msg)
	}
	return a, nil
}

// typing reports whether the active screen has a text input with focus.
func (a App) typing() bool {
	switch a.screen {
	case Words:
		return a.words.filtering
	case Review:
		s := a.current()
		return !s.done() && s.phase == typing
	}
	return false
}

func (a App) updateScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case Home:
		if msg.String() == "enter" {
			a.screen = a.homeAction().target
		}
	case Review:
		return a.updateReview(msg)
	case Words:
		a.words = a.words.update(msg, len(a.filteredWords()), a.wordRows())
	case Learn:
		return a.updateLearn(msg)
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() tea.View {
	w, h := a.size()
	rows := []string{a.header(w)}
	rows = append(rows, a.page(w, max(h-2, 0))...)
	rows = append(rows, a.statusBar(w))
	rows = rows[:min(len(rows), max(h, 1))]

	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}
