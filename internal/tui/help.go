package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// helpSections is the text of the `?` overlay. The README keys table mirrors
// it; change both together.
var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"Everywhere", [][2]string{
		{"tab / shift+tab", "next / previous screen"},
		{":", "command palette: type a screen name, enter to go"},
		{"?", "show / hide this help"},
		{"c", "show / hide the coach (140+ columns)"},
		{"esc", "back to Home"},
		{"q", "quit on Home, back to Home elsewhere"},
		{"ctrl+c", "quit"},
	}},
	{"Home", [][2]string{
		{"enter", "start the preselected action"},
	}},
	{"Review", [][2]string{
		{"space", "reveal the answer (English → Spanish)"},
		{"1–4", "grade your recall: 1 again · 2 hard · 3 good · 4 easy"},
		{"enter", "check a typed answer (Spanish → English)"},
		{"u", "undo the last grade"},
		{"r", "review the session's mistakes again (dates don't move)"},
	}},
	{"Words", [][2]string{
		{"/", "filter by word or meaning (enter keep · esc clear)"},
		{"↑ ↓ / j k", "scroll"},
	}},
}

func (a App) helpView() string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render("Keys") + "\n")
	for _, sec := range helpSections {
		b.WriteString("\n" + r.label.Render(sec.title) + "\n")
		for _, k := range sec.keys {
			fmt.Fprintf(&b, "  %s %s\n", r.accent.Render(fmt.Sprintf("%-16s", k[0])), k[1])
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// coachView is the coach panel's text. In M1 it only reflects the learner's
// own history with the card on screen; tips and "use it today" need the LLM
// and arrive in M3.
func (a App) coachView(w int) string {
	r := a.st.chrome
	title := r.title.Render("Coach") + "\n\n"
	s := a.current()
	if a.screen != Review || s.done() {
		return title + r.label.Render("Your history shows here during review.")
	}

	c := a.cards[s.queue[s.pos]]
	if s.phase == revealed || s.phase == checked {
		// Show the history the card had when it came up, not after this grade.
		if s.undo != nil && s.undo.pos == s.pos {
			c = s.undo.before
		}
	}
	seen := c.TimesCorrect + c.TimesFailed
	lines := []string{
		r.label.Render(c.Direction.String()),
		r.value.Render(fmt.Sprintf("seen %d · ✓ %d · ✗ %d", seen, c.TimesCorrect, c.TimesFailed)),
	}
	if c.LastWrong != "" {
		lines = append(lines, r.value.Render(ansi.Truncate(fmt.Sprintf("last: wrote %q", c.LastWrong), w, "…")))
	}
	return title + strings.Join(lines, "\n")
}
