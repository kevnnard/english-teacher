// Package tui holds the Bubbletea screens of the teacher.
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

// SaveFunc persists a card after the learner grades it.
type SaveFunc func(domain.Card) error

// gradeHint explains the grading keys once the answer is visible.
const gradeHint = "0-2 forgot · 3 hard · 4 good · 5 easy"

// ANSI colors 0–15 follow the terminal theme.
var (
	progressStyle = lipgloss.NewStyle().Faint(true)
	wordStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	pronStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	meaningStyle  = lipgloss.NewStyle().Bold(true)
	exampleStyle  = lipgloss.NewStyle().Italic(true)
	hintStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	boxStyle      = lipgloss.NewStyle().Padding(1, 2)
)

// Model is the review screen: it walks through the due cards one by one.
type Model struct {
	cards    []domain.Card
	current  int  // index of the card on screen
	revealed bool // whether meaning and example are visible
	save     SaveFunc
	now      func() time.Time
	err      error // a failed save stops the session
}

// New returns a review session over cards. Each graded card is rescheduled
// with SM-2 at now() and passed to save before moving on.
func New(cards []domain.Card, save SaveFunc, now func() time.Time) Model {
	return Model{cards: slices.Clone(cards), save: save, now: now}
}

// Init implements tea.Model; the review needs no startup command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Err returns the save error that ended the session, if any.
func (m Model) Err() error {
	return m.err
}

func (m Model) done() bool {
	return m.current >= len(m.cards)
}

// Update implements tea.Model: it reacts to key presses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	key := k.String()

	switch {
	case key == "ctrl+c" || key == "q":
		return m, tea.Quit
	case m.err != nil || m.done():
		return m, tea.Quit // any key leaves the end screen
	case !m.revealed:
		if key == "space" {
			m.revealed = true
		}
		return m, nil
	}

	// The answer is visible: only grades 0–5 do something.
	if len(key) != 1 || key[0] < '0' || key[0] > '5' {
		return m, nil
	}
	grade := domain.Grade(key[0] - '0')

	// Saving synchronously is fine for a local SQLite file and guarantees
	// nothing is lost if the learner quits right after answering.
	card := srs.Review(m.cards[m.current], grade, m.now())
	if err := m.save(card); err != nil {
		m.err = err
		return m, nil
	}
	m.cards[m.current] = card
	m.current++
	m.revealed = false
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var b strings.Builder
	switch {
	case m.err != nil:
		b.WriteString(errorStyle.Render("Could not save your answer: "+m.err.Error()) + "\n\n")
		b.WriteString(hintStyle.Render("press any key to quit"))
	case m.done():
		if n := len(m.cards); n > 0 {
			fmt.Fprintf(&b, "Reviewed %d %s.\n\n", n, plural(n, "card", "cards"))
		}
		b.WriteString("No cards due. See you tomorrow!\n\n")
		b.WriteString(hintStyle.Render("press any key to quit"))
	default:
		c := m.cards[m.current]
		b.WriteString(progressStyle.Render(fmt.Sprintf("%d/%d", m.current+1, len(m.cards))) + "\n\n")
		b.WriteString(wordStyle.Render(c.Word) + "  " + pronStyle.Render(c.Pronunciation) + "\n\n")
		if m.revealed {
			b.WriteString(meaningStyle.Render(c.Meaning) + "\n")
			b.WriteString(exampleStyle.Render(c.Example) + "\n\n")
			b.WriteString(hintStyle.Render(gradeHint + " · q quit"))
		} else {
			b.WriteString(hintStyle.Render("space show answer · q quit"))
		}
	}

	v := tea.NewView(boxStyle.Render(b.String()))
	v.AltScreen = true
	return v
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
