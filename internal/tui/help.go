package tui

import (
	"fmt"
	"strings"
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
		{"space", "reveal the answer"},
		{"0–5", "grade your recall: 0-2 forgot · 3 hard · 4 good · 5 easy"},
		{"enter", "back to Home when the session is done"},
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
// own history; tips and "use it today" need the LLM and arrive in M3.
func (a App) coachView(w int) string {
	r := a.st.chrome
	return r.title.Render("Coach") + "\n\n" + r.label.Render("Your history shows here during review.")
}
