// Package tui holds the Bubbletea app of the teacher: one root model that
// owns the screens, the notebook page layout and the global keys.
package tui

import (
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
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
	Talk
	Fluency
	Quiz
	Listen
	Pairs
	Speak
	Words
	Stats
	Level // the readiness for the next level and the level check; opened with l on Home, not a tab
)

// screens is the order of the tabs and of tab / shift+tab.
var screens = []Screen{Home, Review, Learn, Write, Talk, Fluency, Quiz, Listen, Pairs, Speak, Words, Stats}

func (s Screen) String() string {
	return [...]string{"Home", "Review", "Learn", "Write", "Talk", "Fluency", "Quiz", "Listen", "Pairs", "Speak", "Words", "Stats", "Level"}[s]
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

	Teacher Teacher // the AI teacher; nil turns Learn, Write, Talk and the grammar quiz off
	Add     AddFunc // stores cards the teacher made; nil adds nothing

	Speaker speech.Speaker // reads text aloud; nil when audio is not set up
	Voices  []string       // the installed voice names; with two or more, Pairs alternates them

	Recorder    speech.Recorder    // records the learner for Speak; nil when no recorder is found
	Transcriber speech.Transcriber // writes down what the learner said; nil when whisper is not set up

	PairStats   map[string]domain.PairStat // the learner's totals per minimal-pair contrast
	RecordPairs RecordPairsFunc            // adds a round's results to the totals; nil saves nothing

	MistakeCounts  map[string]int     // logged major writing errors per kind
	RecordMistakes RecordMistakesFunc // logs major writing errors; nil logs nothing

	SaveProfile SaveProfileFunc // stores the profile after a passed level check; nil saves nothing

	Story     domain.Story  // the story being read in Learn; the zero story means none yet
	SaveStory SaveStoryFunc // stores the story after each chapter and answer; nil saves nothing
}

// SaveProfileFunc stores the learner profile, like store.SaveProfile.
type SaveProfileFunc func(domain.Profile) error

// RecordMistakesFunc logs the major errors of one text, like
// store.RecordMistakes.
type RecordMistakesFunc func([]domain.Mistake) error

// RecordPairsFunc adds the results of one minimal-pairs round to the
// learner's totals, like store.RecordPairResults.
type RecordPairsFunc func([]domain.PairStat) error

// DefaultMaxNew is the usual cap on new cards in one session.
const DefaultMaxNew = 20

// App is the root Bubbletea model.
type App struct {
	cards []domain.Card
	save  SaveFunc
	now   func() time.Time

	profile     domain.Profile
	saveProfile SaveProfileFunc
	check       levelCheckState // the level check, when one is running or just ended
	days        []domain.Day
	record      RecordFunc
	lastStudy   time.Time     // when the learner last graded a card
	studied     time.Duration // study time not yet recorded as a whole minute

	teacher        Teacher
	add            AddFunc
	learn          learnState
	story          storyState    // the story in Learn, read chapter by chapter
	saveStoryFn    SaveStoryFunc // nil saves nothing
	write          writeState
	talk           talkState
	talkQuiet      bool // voice mode is off: Talk replies are not read aloud
	fluency        fluencyState
	quiz           quizState
	mistakeCounts  map[string]int // major writing errors per kind, as stored
	recordMistakes RecordMistakesFunc

	speaker   speech.Speaker // nil when audio is not set up
	listen    listenState
	dictation dictationState
	long      longState // the long listening in Listen
	voices    []string  // installed voice names, for Pairs

	recorder    speech.Recorder    // nil when no recorder is found
	transcriber speech.Transcriber // nil when whisper is not set up
	speaking    speakState

	pairs       pairsState
	pairStats   map[string]domain.PairStat // totals per contrast, as stored
	recordPairs RecordPairsFunc

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
		cards:   cards,
		save:    cfg.Save,
		now:     cfg.Now,
		profile: profile,
		days:    slices.Clone(cfg.Days),
		record:  cfg.Record,
		teacher: cfg.Teacher,
		add:     cfg.Add,
		speaker: cfg.Speaker,
		voices:  slices.Clone(cfg.Voices),

		recorder:    cfg.Recorder,
		transcriber: cfg.Transcriber,

		pairStats:   maps.Clone(cfg.PairStats),
		recordPairs: cfg.RecordPairs,

		mistakeCounts:  maps.Clone(cfg.MistakeCounts),
		recordMistakes: cfg.RecordMistakes,
		saveProfile:    cfg.SaveProfile,
		story:          storyState{saved: cfg.Story},
		saveStoryFn:    cfg.SaveStory,
		lastStudy:      cfg.Now(),
		screen:         cfg.Start,
		st:             newStyles(true), // until the terminal reports its background
		notes:          true,
		review:         newSession(cards, queue, false, cfg.Now()),
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
		m, cmd := a.handleKey(msg)
		if next, ok := m.(App); ok {
			return next.stopLongWhenAway(), cmd // esc, tab or : left the screen
		}
		return m, cmd
	case listeningMsg:
		return a.gotListening(msg)
	case lineSpokenMsg:
		return a.gotLineSpoken(msg)
	case readingMsg:
		return a.gotReading(msg), nil
	case chapterMsg:
		return a.gotChapter(msg), nil
	case taskMsg:
		return a.gotTask(msg), nil
	case correctionMsg:
		return a.gotCorrection(msg), nil
	case judgeMsg:
		return a.gotJudge(msg), nil
	case talkMsg:
		return a.gotTurn(msg)
	case talkFeedbackMsg:
		return a.gotTalkFeedback(msg), nil
	case fluencyTickMsg:
		return a.gotFluencyTick(msg)
	case quizGrammarMsg:
		return a.gotGrammar(msg), nil
	case quizClozeMsg:
		return a.gotCloze(msg), nil
	case checkReadingMsg:
		return a.gotCheckReading(msg), nil
	case spokenMsg:
		return a.gotSpoken(msg), nil
	case transcribedMsg:
		if msg.talk {
			return a.gotTalkTranscript(msg), nil
		}
		return a.gotTranscript(msg), nil
	}
	return a, nil
}

// handleKey routes a key press: overlays and text inputs first, then the
// global keys, then the active screen.
func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	a.listen.notice = "" // an audio notice lasts until the next key
	switch {
	case k == "ctrl+c":
		// A recorder must not outlive the app.
		for _, rec := range []speech.Recording{a.speaking.recording, a.talk.recording} {
			if rec != nil {
				rec.Cancel()
			}
		}
		a.long.stop() // and neither must a voice
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
	if a, cmd, ok := a.listenKey(k); ok {
		return a, cmd
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
	case Write:
		return a.teacher != nil && a.write.typing()
	case Talk:
		return a.teacher != nil && a.talk.typing()
	case Fluency:
		return a.fluency.typing()
	case Quiz:
		return a.quiz.typing()
	case Listen:
		return !a.long.on && a.dictation.typing()
	case Speak:
		return a.speaking.typing()
	}
	return false
}

func (a App) updateScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case Home:
		switch msg.String() {
		case "enter":
			a.screen = a.homeAction().target
		case "l":
			a.screen = Level
		}
	case Level:
		return a.updateLevel(msg)
	case Review:
		return a.updateReview(msg)
	case Words:
		a.words = a.words.update(msg, len(a.filteredWords()), a.wordRows())
	case Learn:
		return a.updateLearn(msg)
	case Write:
		return a.updateWrite(msg)
	case Talk:
		return a.updateTalk(msg)
	case Fluency:
		return a.updateFluency(msg)
	case Quiz:
		return a.updateQuiz(msg)
	case Listen:
		if a.long.on {
			return a.updateLong(msg)
		}
		if msg.String() == "s" && !a.dictation.typing() {
			return a.toggleLong() // while typing, s is a letter of the answer
		}
		return a.updateDictation(msg)
	case Pairs:
		return a.updatePairs(msg)
	case Speak:
		return a.updateSpeak(msg)
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
