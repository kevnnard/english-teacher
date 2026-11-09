package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// wordList is the state of the Words screen.
type wordList struct {
	filter    string
	filtering bool // the filter input has focus
	offset    int  // first visible row
}

// update handles a key on the Words screen. total is the number of rows that
// match the filter and visible how many fit on screen; they bound scrolling.
func (l wordList) update(msg tea.KeyPressMsg, total, visible int) wordList {
	k := msg.String()
	if l.filtering {
		switch k {
		case "enter":
			l.filtering = false
		case "esc":
			l.filter, l.filtering = "", false
		case "backspace":
			l.filter = dropLastRune(l.filter)
		default:
			l.filter += msg.Text
		}
		l.offset = 0
		return l
	}
	switch k {
	case "/":
		l.filtering = true
	case "down", "j":
		l.offset = min(l.offset+1, max(total-visible, 0))
	case "up", "k":
		l.offset = max(l.offset-1, 0)
	}
	return l
}

// wordCards are the cards of the Words screen and Home counts: one per
// word, its recognition card, since every word starts there.
func (a App) wordCards() []domain.Card {
	var out []domain.Card
	for _, c := range a.cards {
		if c.Direction == domain.Recognition {
			out = append(out, c)
		}
	}
	return out
}

// filteredWords are the word cards whose word or meaning contains the filter.
func (a App) filteredWords() []domain.Card {
	q := strings.ToLower(a.words.filter)
	var out []domain.Card
	for _, c := range a.wordCards() {
		if strings.Contains(strings.ToLower(c.Word), q) || strings.Contains(strings.ToLower(c.Meaning), q) {
			out = append(out, c)
		}
	}
	return out
}

// wordRows is how many table rows fit under the header line.
func (a App) wordRows() int {
	_, h := a.paneSize()
	return max(h-2, 1)
}

func (a App) wordsView() string {
	r := a.st.main
	var b strings.Builder
	switch {
	case a.words.filtering:
		b.WriteString(r.accent.Render("/ ") + a.words.filter + r.accent.Render("▏"))
	case a.words.filter != "":
		b.WriteString(r.label.Render("/ ") + a.words.filter)
	default:
		b.WriteString(r.label.Render("/ filter"))
	}
	b.WriteString("\n\n")

	rows := a.filteredWords()
	if len(rows) == 0 {
		b.WriteString(r.label.Render("No words match."))
		return b.String()
	}
	start := min(a.words.offset, max(len(rows)-a.wordRows(), 0))
	end := min(start+a.wordRows(), len(rows))
	now := a.now()
	for _, c := range rows[start:end] {
		b.WriteString(a.wordRow(c, now) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// wordRow is "● due        hello                  Hola".
func (a App) wordRow(c domain.Card, now time.Time) string {
	r := a.st.main
	s := c.Status(now)
	return a.glyph(s) + " " + r.secondary.Render(fmt.Sprintf("%-10s", s)) + " " +
		r.target.Render(fmt.Sprintf("%-22s", c.Word)) + " " + r.value.Render(c.Meaning)
}

func dropLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
