package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// layout is how the screen is split, chosen from the terminal width
// (design/DESIGN.md §4).
type layout int

const (
	focusLayout layout = iota // one pane, progress line on top
	railLayout                // rail + content
	wideLayout                // rail + content + coach
)

const (
	railWidth     = 26  // including its rule
	coachWidth    = 44  // including its rule
	railMinWidth  = 100 // narrower terminals get the focus layout
	coachMinWidth = 140 // wider terminals get room for the coach

	// Used until the terminal reports its size.
	defaultWidth  = 80
	defaultHeight = 24

	inset = 2 // columns between a region's edge and its text
)

func (a App) size() (w, h int) {
	if a.width == 0 || a.height == 0 {
		return defaultWidth, defaultHeight
	}
	return a.width, a.height
}

func (a App) layout() layout {
	w, _ := a.size()
	switch {
	case w < railMinWidth:
		return focusLayout
	case w >= coachMinWidth && a.coach:
		return wideLayout
	default:
		return railLayout
	}
}

// block turns s into exactly h lines of exactly w columns: longer lines are
// cut, shorter ones padded with pad's background. This is what guarantees
// no line is ever wider than the terminal.
func block(s string, w, h int, pad lipgloss.Style) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, h)
	for i := range out {
		var l string
		if i < len(lines) {
			l = ansi.Truncate(lines[i], w, "…")
		}
		if gap := w - ansi.StringWidth(l); gap > 0 {
			l += pad.Render(strings.Repeat(" ", gap))
		}
		out[i] = l
	}
	return out
}

// joinColumns glues equally tall blocks side by side.
func joinColumns(cols ...[]string) []string {
	out := make([]string, len(cols[0]))
	for i := range out {
		var b strings.Builder
		for _, c := range cols {
			b.WriteString(c[i])
		}
		out[i] = b.String()
	}
	return out
}

// indent prefixes every line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// paneSize is the room the active screen has for its own text.
func (a App) paneSize() (w, h int) {
	tw, th := a.size()
	body := max(th-1, 0)
	switch a.layout() {
	case focusLayout:
		w, h = tw, body-1
	case railLayout:
		w, h = tw-railWidth, body
	case wideLayout:
		w, h = tw-railWidth-coachWidth, body
	}
	// One blank row on top and an inset on both sides.
	return max(w-2*inset, 1), max(h-1, 0)
}

// pane renders the content region: an overlay or the active screen.
func (a App) pane(w, h int) []string {
	iw, ih := a.paneSize()
	var s string
	switch {
	case a.err != nil:
		s = a.errorView()
	case a.help:
		s = a.helpView()
	case a.palette.open:
		s = a.paletteView()
	default:
		s = a.screenView(iw, ih)
	}
	return block("\n"+indent(s, inset), w, h, a.st.main.base)
}

func (a App) screenView(w, h int) string {
	switch a.screen {
	case Home:
		return a.homeView()
	case Review:
		return a.reviewView(w)
	case Words:
		return a.wordsView()
	case Stats:
		return a.statsView()
	default:
		return a.st.main.title.Render(a.screen.String()) + a.st.main.label.Render(" · Coming in M3")
	}
}

func (a App) errorView() string {
	r := a.st.main
	return r.danger.Render("✗ Could not save your answer: "+a.err.Error()) + "\n\n" +
		r.label.Render("Your earlier answers are saved. Press any key to quit.")
}

// rail is the left navigation column with its rule.
func (a App) rail(h int) []string {
	r := a.st.chrome
	counts := map[Screen]string{Words: fmt.Sprint(len(a.wordCards()))}
	if n := a.review.remaining(); n > 0 {
		counts[Review] = "● " + fmt.Sprint(n)
	}

	lines := []string{""}
	for _, s := range screens {
		name, count := "   "+s.String(), counts[s]
		style := r.value
		if s == a.screen {
			name, style = " ▸ "+s.String(), r.title
		}
		gap := max(railWidth-1-inset-ansi.StringWidth(name)-ansi.StringWidth(count), 1)
		countStyle := r.label
		if s == Review {
			countStyle = r.accent
		}
		lines = append(lines, style.Render(name)+r.base.Render(strings.Repeat(" ", gap))+countStyle.Render(count))
	}

	text := block(strings.Join(lines, "\n"), railWidth-1, h, r.base)
	for i := range text {
		text[i] += r.rule.Render("│")
	}
	return text
}

// coachPanel is the right column with its rule.
func (a App) coachPanel(h int) []string {
	r := a.st.chrome
	s := "\n" + indent(a.coachView(coachWidth-1-2*inset), inset)
	text := block(s, coachWidth-1, h, r.base)
	for i := range text {
		text[i] = r.rule.Render("│") + text[i]
	}
	return text
}

// progressLine shows how much of today's session is done, for the focus layout.
func (a App) progressLine(w int) string {
	done, total := a.review.pos, len(a.review.queue)
	filled := 0
	if total > 0 {
		filled = w * done / total
	}
	r := a.st.main
	return r.accent.Render(strings.Repeat("━", filled)) + r.rule.Render(strings.Repeat("─", w-filled))
}

// statusBar is the bottom row: facts on the left, key hints on the right.
// Hints are dropped from the end when the terminal is too narrow.
func (a App) statusBar(w int) string {
	r := a.st.chrome
	sep := r.label.Render(" │ ")
	facts := []string{r.accent.Render("A1"), r.value.Render(fmt.Sprintf("%d due", a.review.remaining()))}
	if s := a.review; s.correct+s.failed > 0 {
		facts = append(facts, r.success.Render(fmt.Sprintf("✓ %d", s.correct))+r.base.Render(" ")+
			r.danger.Render(fmt.Sprintf("✗ %d", s.failed)))
	}
	left := r.base.Render(" ") + strings.Join(facts, sep)

	hints := a.hints()
	if w >= coachMinWidth {
		hints = append(hints, "c coach")
	}
	right := ""
	for n := len(hints); n > 0; n-- {
		right = r.label.Render(strings.Join(hints[:n], " · ") + " ")
		if ansi.StringWidth(left)+ansi.StringWidth(right)+1 <= w {
			break
		}
		right = ""
	}

	gap := max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 0)
	return block(left+r.base.Render(strings.Repeat(" ", gap))+right, w, 1, r.base)[0]
}

// hints are the keys worth showing for what is on screen now.
func (a App) hints() []string {
	switch {
	case a.err != nil:
		return []string{"any key quit"}
	case a.help:
		return []string{"? close", "esc close"}
	case a.palette.open:
		return []string{"type to filter", "enter go", "esc close"}
	}
	switch a.screen {
	case Home:
		return []string{"enter start", "tab screens", ": palette", "? help", "q quit"}
	case Review:
		return a.reviewHints()
	case Words:
		if a.words.filtering {
			return []string{"type to filter", "enter keep", "esc clear"}
		}
		return []string{"/ filter", "↑↓ scroll", "esc home", "? help"}
	default:
		return []string{"tab screens", ": palette", "esc home", "? help"}
	}
}
