package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

// cardWidth is the widest the review card gets, borders included.
const cardWidth = 52

// session is one review sitting: the cards due when the app opened.
type session struct {
	queue    []int // indices into App.cards, in review order
	pos      int   // how many cards of queue are done
	revealed bool  // whether the current card's answer is visible

	correct, failed int
}

func newSession(cards []domain.Card, now time.Time) session {
	var s session
	for i, c := range cards {
		if !c.Due.After(now) {
			s.queue = append(s.queue, i)
		}
	}
	return s
}

func (s session) remaining() int { return len(s.queue) - s.pos }

func (s session) done() bool { return s.pos >= len(s.queue) }

func (a App) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &a.review
	k := msg.String()
	switch {
	case s.done():
		if k == "enter" {
			a.screen = Home
		}
	case !s.revealed:
		s.revealed = k == "space"
	case len(k) == 1 && k[0] >= '0' && k[0] <= '5':
		grade := domain.Grade(k[0] - '0')
		idx := s.queue[s.pos]
		// Saving synchronously is fine for a local SQLite file and guarantees
		// nothing is lost if the learner quits right after answering.
		card := srs.Review(a.cards[idx], grade, a.now())
		if err := a.save(card); err != nil {
			a.err = err
			return a, nil
		}
		a.cards[idx] = card
		if grade.Passed() {
			s.correct++
		} else {
			s.failed++
		}
		s.pos++
		s.revealed = false
	}
	return a, nil
}

func (a App) reviewHints() []string {
	switch s := a.review; {
	case s.done():
		return []string{"enter home", "? help"}
	case !s.revealed:
		return []string{"space reveal", "esc home", "? help"}
	default:
		return []string{"0-5 grade", "esc home", "? help"}
	}
}

func (a App) reviewView(w int) string {
	r := a.st.main
	s := a.review
	if len(s.queue) == 0 {
		return r.secondary.Render("No reviews due — come back tomorrow.")
	}
	if s.done() {
		n := len(s.queue)
		return r.target.Render("Done.") + " " + r.secondary.Render(fmt.Sprintf("%d reviewed.", n)) + "   " +
			r.success.Render(fmt.Sprintf("✓ %d", s.correct)) + "   " + r.danger.Render(fmt.Sprintf("✗ %d", s.failed))
	}

	c := a.cards[s.queue[s.pos]]
	width := min(cardWidth, w)
	header := r.label.Render("Recognize · English → Spanish")
	counter := r.label.Render(fmt.Sprintf("%d / %d", s.pos+1, len(s.queue)))
	gap := max(width-ansi.StringWidth(header)-ansi.StringWidth(counter), 2)

	var back []cardLine
	if s.revealed {
		back = []cardLine{{c.Meaning, r.value}, {`"` + c.Example + `"`, r.secondary.Italic(true)}}
	}
	hint := r.label.Render("space reveal")
	if s.revealed {
		hint = r.danger.Render("0-2") + " forgot   " + r.warning.Render("3") + " hard   " +
			r.title.Render("4 good") + "   " + r.success.Render("5") + " easy"
	}
	return header + strings.Repeat(" ", gap) + counter + "\n\n" +
		a.cardBox(width, []cardLine{{c.Word, r.target}, {c.Pronunciation, r.meta}}, back) + "\n" +
		"  " + hint
}

// cardLine is one line of text on the review card with its style. Text is
// kept plain until it has been wrapped or cut to fit.
type cardLine struct {
	text  string
	style lipgloss.Style
}

// cardBox draws the review card: front lines centered on top, back lines
// below a divider. Long back lines wrap inside the card; the front never
// wraps, it is cut instead (DESIGN.md §8).
func (a App) cardBox(width int, front, back []cardLine) string {
	border := a.st.main.accent
	inner := width - 2 // inside the borders
	text := inner - 2*inset

	row := func(left int, l cardLine) string {
		s := strings.Repeat(" ", left) + l.style.Render(l.text)
		return border.Render("│") + s + strings.Repeat(" ", max(inner-ansi.StringWidth(s), 0)) + border.Render("│")
	}
	rule := func(l, r string) string { return border.Render(l + strings.Repeat("─", inner) + r) }

	lines := []string{rule("╭", "╮"), row(0, cardLine{})}
	for _, f := range front {
		f.text = ansi.Truncate(f.text, text, "…")
		lines = append(lines, row((inner-ansi.StringWidth(f.text))/2, f))
	}
	lines = append(lines, row(0, cardLine{}), rule("├", "┤"))
	if len(back) == 0 {
		back = []cardLine{{}, {}} // keep the card the same height before the reveal
	}
	for _, b := range back {
		for _, l := range strings.Split(ansi.Wordwrap(b.text, text, ""), "\n") {
			lines = append(lines, row(inset, cardLine{ansi.Truncate(l, text, "…"), b.style}))
		}
	}
	lines = append(lines, rule("╰", "╯"))
	return strings.Join(lines, "\n")
}
