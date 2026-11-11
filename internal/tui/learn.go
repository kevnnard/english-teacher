package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// Teacher is the AI teacher the Learn, Write and Talk screens and long
// listening in Listen use.
// *teacher.Teacher implements it; tests use a fake.
type Teacher interface {
	Reading(ctx context.Context, level domain.Level, known []string) (teacher.Reading, error)
	WritingTask(ctx context.Context, level domain.Level, recent, focus []string) (teacher.WritingTask, error)
	Correct(ctx context.Context, level domain.Level, task, text string) (teacher.Correction, error)
	Judge(ctx context.Context, level domain.Level, attempts []teacher.Attempt) ([]bool, error)
	RolePlay(ctx context.Context, level domain.Level, s teacher.Scenario, lines []teacher.Line) (teacher.Turn, error)
	RolePlayFeedback(ctx context.Context, level domain.Level, s teacher.Scenario, lines []teacher.Line) (teacher.Feedback, error)
	GrammarQuiz(ctx context.Context, level domain.Level, focus []string) (teacher.GrammarQuiz, error)
	CheckReading(ctx context.Context, level domain.Level) (teacher.CheckReading, error)
	StoryChapter(ctx context.Context, level domain.Level, known []string, so teacher.StorySoFar) (teacher.StoryChapter, error)
	ListeningText(ctx context.Context, level domain.Level, known []string, kind teacher.ListeningKind) (teacher.Listening, error)
	ClozeRound(ctx context.Context, level domain.Level, mistakes, sentences []string) (teacher.ClozeQuiz, error)
}

// AddFunc stores new cards and returns how many words were new.
type AddFunc func([]domain.Card) (int, error)

// maxKnownWords caps the known words sent with a reading request, so the
// prompt stays short as the deck grows.
const maxKnownWords = 80

// maxReadingMinutes caps the study time one reading can record, so a text
// left open over lunch does not fill the daily goal.
const maxReadingMinutes = 15

type learnPhase int

const (
	learnIdle    learnPhase = iota // nothing asked yet
	learnLoading                   // waiting for the teacher
	learnFailed                    // the teacher could not write a text
	learnReading                   // a text is on screen
	learnDone                      // the marked words were added
)

// learnState is the Learn screen: one graded reading and its glossary.
type learnState struct {
	phase   learnPhase
	req     int // the id of the latest request; older answers are ignored
	reading teacher.Reading
	marked  []bool // marked[i]: add glossary entry i as a card
	cursor  int
	shownAt time.Time // when the text appeared, for the study minutes
	added   int
	err     error
}

// readingMsg is the teacher's answer to request req.
type readingMsg struct {
	req     int
	reading teacher.Reading
	err     error
}

// askReading starts a new reading request.
func (a App) askReading() (App, tea.Cmd) {
	if a.teacher == nil {
		return a, nil
	}
	a.learn = learnState{phase: learnLoading, req: a.learn.req + 1}
	req, tc, level, known := a.learn.req, a.teacher, a.profile.Level, a.knownWords()
	return a, func() tea.Msg {
		r, err := tc.Reading(context.Background(), level, known)
		return readingMsg{req: req, reading: r, err: err}
	}
}

// gotReading shows the answer to the latest request.
func (a App) gotReading(msg readingMsg) App {
	if msg.req != a.learn.req || a.learn.phase != learnLoading {
		return a
	}
	if msg.err != nil {
		a.learn.phase, a.learn.err = learnFailed, msg.err
		return a
	}
	a.learn.phase, a.learn.reading, a.learn.shownAt = learnReading, msg.reading, a.now()
	a.learn.marked = make([]bool, len(msg.reading.Glossary))
	for i, g := range msg.reading.Glossary {
		a.learn.marked[i] = g.New
	}
	return a
}

// knownWords are the distinct words of the deck, first imported first.
func (a App) knownWords() []string {
	seen := map[string]bool{}
	var words []string
	for _, c := range a.cards {
		k := strings.ToLower(c.Word)
		if !seen[k] && len(words) < maxKnownWords {
			seen[k] = true
			words = append(words, c.Word)
		}
	}
	return words
}

func (a App) updateLearn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	l := &a.learn
	k := msg.String()
	if k == "s" && l.phase != learnLoading && a.story.phase != storyLoading {
		return a.toggleStory()
	}
	if a.story.on {
		return a.updateStory(msg)
	}
	switch {
	case k == "enter" && (l.phase == learnIdle || l.phase == learnFailed):
		return a.askReading()
	case k == "n" && (l.phase == learnReading || l.phase == learnDone || l.phase == learnFailed):
		return a.askReading()
	case l.phase != learnReading:
	case k == "down" || k == "j":
		l.cursor = min(l.cursor+1, max(len(l.marked)-1, 0))
	case k == "up" || k == "k":
		l.cursor = max(l.cursor-1, 0)
	case k == "space" && len(l.marked) > 0:
		l.marked[l.cursor] = !l.marked[l.cursor]
	case k == "a":
		a.finishReading()
	}
	return a, nil
}

// finishReading adds the marked words and records the reading as study.
func (a *App) finishReading() {
	l := &a.learn
	n, err := a.addGlossary(l.reading.Glossary, l.marked)
	if err != nil {
		l.err = err
		return
	}
	l.added = n
	minutes := min(int(a.now().Sub(l.shownAt)/time.Minute), maxReadingMinutes)
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{"Reading: " + l.reading.Title}})
	l.phase, l.err = learnDone, nil
}

// addGlossary stores the glossary entries marked for study as new cards
// and puts them in the deck. It returns how many words were new.
func (a *App) addGlossary(glossary []teacher.Gloss, marked []bool) (int, error) {
	var cards []domain.Card
	for i, g := range glossary {
		if marked[i] {
			cards = append(cards, g.Card())
		}
	}
	if len(cards) == 0 || a.add == nil {
		return 0, nil
	}
	n, err := a.add(cards)
	if err != nil {
		return 0, err
	}
	a.addCards(cards)
	return n, nil
}

// addCards puts cards the store just added into the deck, in both
// directions, so Words and the counts show them at once. They are studied
// from the next session, which plans its queue on start.
func (a *App) addCards(cards []domain.Card) {
	have := map[string]bool{}
	for _, c := range a.cards {
		have[strings.ToLower(c.Word)] = true
	}
	for _, c := range cards {
		if have[strings.ToLower(c.Word)] {
			continue
		}
		have[strings.ToLower(c.Word)] = true
		rec, prod := c, c
		rec.Direction, rec.LastWrong = domain.Recognition, ""
		prod.Direction = domain.Production
		a.cards = append(a.cards, rec, prod)
	}
}

func (a App) learnView(w, h int) string {
	r := a.st.main
	l := a.learn
	var b strings.Builder
	switch {
	case a.teacher == nil:
		b.WriteString(r.title.Render("Learn") + "\n\n")
		b.WriteString(r.secondary.Render("The AI teacher is off. Install Gentle Shell (the pi command) and see `teacher models`."))
		return b.String()
	case a.story.on:
		return a.storyView(w, h)
	case l.phase == learnIdle:
		b.WriteString(r.title.Render("Learn") + "\n\n")
		b.WriteString(r.target.Render("Read a short text written for you.") + "\n")
		b.WriteString(r.secondary.Render(fmt.Sprintf("It uses the words you know and at most %d new ones,", teacher.MaxNewWords)) + "\n")
		b.WriteString(r.secondary.Render("so you understand almost everything and learn from context.") + "\n\n")
		b.WriteString(r.accent.Render("enter") + r.secondary.Render(" a short text   ") +
			r.accent.Render("s") + r.secondary.Render(" a story in chapters, at your level"))
		return b.String()
	case l.phase == learnLoading:
		b.WriteString(r.title.Render("Learn") + "\n\n")
		b.WriteString(r.secondary.Render("Writing a text for you… this takes about 10 seconds."))
		return b.String()
	case l.phase == learnFailed:
		b.WriteString(r.title.Render("Learn") + "\n\n")
		b.WriteString(r.danger.Render("✗ Could not get a text: "+l.err.Error()) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
		return b.String()
	}

	b.WriteString(r.title.Render(l.reading.Title) + "\n\n")
	text := ansi.Wordwrap(strings.TrimSpace(l.reading.Text), max(w, 20), "")
	b.WriteString(r.target.Render(text) + "\n\n")
	used := strings.Count(text, "\n") + 6

	if l.phase == learnDone {
		msg := "Done. "
		if l.added > 0 {
			msg += fmt.Sprintf("Added %d new words to review.", l.added)
		} else {
			msg += "No new words added."
		}
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render(msg))
		return b.String()
	}
	if l.err != nil {
		b.WriteString(r.danger.Render("✗ Could not add the words: "+l.err.Error()) + "\n\n")
		used += 2
	}

	b.WriteString(r.label.Render("Glossary · mark the words to study") + "\n")
	rows := max(h-used, 3)
	first := max(min(l.cursor-rows/2, len(l.marked)-rows), 0)
	for i := first; i < min(first+rows, len(l.marked)); i++ {
		g := l.reading.Glossary[i]
		pointer, box := "  ", "[ ] "
		if i == l.cursor {
			pointer = r.accent.Render("▸ ")
		}
		if l.marked[i] {
			box = "[x] "
		}
		line := pointer + r.value.Render(box+g.Word) + r.secondary.Render(" — "+g.Meaning)
		if g.New {
			line += r.accent.Render("  new")
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a App) learnHints() []string {
	switch {
	case a.teacher == nil:
		return []string{"tab screens", "esc home", "? help"}
	case a.story.on:
		return a.storyHints()
	case a.learn.phase == learnIdle:
		return []string{"enter read", "s story", "tab screens", "esc home", "? help"}
	case a.learn.phase == learnLoading:
		return []string{"tab screens", "esc home"}
	case a.learn.phase == learnFailed:
		return []string{"enter try again", "esc home"}
	case a.learn.phase == learnDone:
		return []string{"n new text", "s story", "tab screens", "esc home"}
	}
	return []string{"↑↓ move", "space mark", "a done", "n new text", "s story", "esc home"}
}
