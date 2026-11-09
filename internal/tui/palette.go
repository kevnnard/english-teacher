package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// palette is the `:` command palette: type to filter screens, enter to go.
type palette struct {
	open     bool
	query    string
	selected int // index into matches()
}

// matches are the screens whose name contains the query, ignoring case.
func (p palette) matches() []Screen {
	q := strings.ToLower(p.query)
	var out []Screen
	for _, s := range screens {
		if strings.Contains(strings.ToLower(s.String()), q) {
			out = append(out, s)
		}
	}
	return out
}

func (a App) updatePalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &a.palette
	switch msg.String() {
	case "esc":
		*p = palette{}
	case "enter":
		if m := p.matches(); len(m) > 0 {
			a.screen = m[p.selected]
		}
		*p = palette{}
	case "up":
		p.selected = max(p.selected-1, 0)
	case "down":
		p.selected = min(p.selected+1, max(len(p.matches())-1, 0))
	case "backspace":
		p.query, p.selected = dropLastRune(p.query), 0
	default:
		if msg.Text != "" {
			p.query, p.selected = p.query+msg.Text, 0
		}
	}
	return a, nil
}

func (a App) paletteView() string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.accent.Render(": ") + a.palette.query + r.accent.Render("▏") + "\n\n")
	m := a.palette.matches()
	if len(m) == 0 {
		b.WriteString(r.label.Render("No screen matches."))
	}
	for i, s := range m {
		if i == a.palette.selected {
			b.WriteString(r.title.Render("▸ "+s.String()) + "\n")
		} else {
			b.WriteString("  " + s.String() + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
