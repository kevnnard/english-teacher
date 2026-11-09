// Package tui holds the Bubbletea app of the teacher: one root model that
// owns the screens, the adaptive layout and the global keys.
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

// screens is the order of the rail and of tab / shift+tab.
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
}

// DefaultMaxNew is the usual cap on new cards in one session.
const DefaultMaxNew = 20

// App is the root Bubbletea model.
type App struct {
	cards []domain.Card
	save  SaveFunc
	now   func() time.Time

	screen        Screen
	width, height int // terminal size; zero until the first WindowSizeMsg
	st            styles
	coach         bool // the learner wants the coach panel (shown only when wide)
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
	return App{
		cards:  cards,
		save:   cfg.Save,
		now:    cfg.Now,
		screen: cfg.Start,
		st:     newStyles(true), // until the terminal reports its background
		coach:  true,
		review: newSession(cards, queue, false, cfg.Now()),
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
		a.coach = !a.coach
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
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() tea.View {
	w, h := a.size()
	body := max(h-1, 0) // the last row is the status bar

	var rows []string
	switch a.layout() {
	case focusLayout:
		rows = append([]string{a.progressLine(w)}, a.pane(w, max(body-1, 0))...)
	case railLayout:
		rows = joinColumns(a.rail(body), a.pane(w-railWidth, body))
	case wideLayout:
		rows = joinColumns(a.rail(body), a.pane(w-railWidth-coachWidth, body), a.coachPanel(body))
	}
	rows = append(rows[:min(len(rows), body)], a.statusBar(w))

	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	return v
}
