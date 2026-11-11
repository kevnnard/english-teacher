package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Every screen is a notebook page (the notebook design): a top bar with the
// date, the page context and the tabs; a margin line down the left; the
// status bar at the bottom.
const (
	wideMarginMin = 100 // terminals this wide get the wide margin
	wideMargin    = 6   // columns left of the margin line on wide terminals
	narrowMargin  = 2   // columns left of the margin line otherwise
	gutter        = 3   // columns between the margin line and the text
	pageWidth     = 76  // the widest the text of a page gets

	// Used until the terminal reports its size.
	defaultWidth  = 80
	defaultHeight = 24
)

func (a App) size() (w, h int) {
	if a.width == 0 || a.height == 0 {
		return defaultWidth, defaultHeight
	}
	return a.width, a.height
}

// margin is how many columns sit left of the margin line.
func (a App) margin() int {
	if w, _ := a.size(); w >= wideMarginMin {
		return wideMargin
	}
	return narrowMargin
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

// indent prefixes every line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// paneSize is the room the active screen has for its own text.
func (a App) paneSize() (w, h int) {
	tw, th := a.size()
	w = min(tw-a.margin()-1-2*gutter, pageWidth)
	// The top bar, the status bar and one blank row on top of the page.
	return max(w, 1), max(th-3, 0)
}

// page renders the body between the two bars: the margin line and, right
// of it, an overlay or the active screen.
func (a App) page(w, h int) []string {
	r := a.st.main
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
	left := r.base.Render(strings.Repeat(" ", a.margin())) + r.margin.Render("│")
	text := block("\n"+indent(s, gutter), max(w-a.margin()-1, 0), h, r.base)
	if n := a.listen.notice; n != "" && len(text) > 0 {
		// The audio notice takes the last row of the page.
		pad := strings.Repeat(" ", gutter)
		text[len(text)-1] = block(pad+r.warning.Render(n), max(w-a.margin()-1, 0), 1, r.base)[0]
	}
	for i := range text {
		text[i] = left + text[i]
	}
	return text
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
	case Learn:
		return a.learnView(w, h)
	case Write:
		return a.writeView(w, h)
	case Talk:
		return a.talkView(w, h)
	case Fluency:
		return a.fluencyView(w, h)
	case Quiz:
		return a.quizView(w)
	case Listen:
		if a.long.on {
			return a.longView(w, h)
		}
		return a.dictationView(w)
	case Pairs:
		return a.pairsView(w)
	case Speak:
		return a.speakView(w)
	case Level:
		return a.levelView(w)
	default:
		return a.st.main.title.Render(a.screen.String())
	}
}

func (a App) errorView() string {
	r := a.st.main
	return r.danger.Render("✗ Could not save your answer: "+a.err.Error()) + "\n\n" +
		r.label.Render("Your earlier answers are saved. Press any key to quit.")
}

// header is the top bar: the date and what this page is on the left, the
// tabs on the right. When the tabs do not fit, they move closer together.
// If they still do not fit and the page context only repeats the active
// tab's name, the context goes and then the date gets shorter, down to
// the weekday: on 80 columns every tab stays. Otherwise only the active
// tab stays.
func (a App) header(w int) string {
	r := a.st.chrome
	date := func(layout string) string { return r.base.Render(" ") + r.title.Render(a.now().Format(layout)) }
	pageName := a.pageContext()
	left := date("Monday 2 Jan") + r.label.Render("  ·  ") + r.secondary.Render(pageName)

	tab := func(s Screen) string {
		if s == a.screen {
			return r.title.Render(s.String())
		}
		return r.label.Render(s.String())
	}
	tabs := make([]string, len(screens))
	for i, s := range screens {
		tabs[i] = tab(s)
	}
	fits := func(right string) bool { return ansi.StringWidth(left)+ansi.StringWidth(right)+2 <= w }
	right := strings.Join(tabs, r.base.Render("  ")) + r.base.Render(" ")
	if !fits(right) {
		right = strings.Join(tabs, r.base.Render(" ")) + r.base.Render(" ")
	}
	for _, layout := range []string{"Monday 2 Jan", "Mon 2 Jan", "Mon"} {
		if !fits(right) && pageName == a.screen.String() {
			left = date(layout) // the active tab already names the page
		}
	}
	if !fits(right) {
		left = date("Monday 2 Jan") + r.label.Render("  ·  ") + r.secondary.Render(pageName)
		right = tab(a.screen) + r.base.Render(" ")
	}

	gap := max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return block(left+r.base.Render(strings.Repeat(" ", gap))+right, w, 1, r.base)[0]
}

// pageContext names the page: its number during a review, else the screen.
func (a App) pageContext() string {
	if a.screen != Review || a.err != nil || a.help || a.palette.open {
		return a.screen.String()
	}
	s := a.current()
	switch {
	case len(s.queue) == 0:
		return "Review"
	case s.done():
		return "Summary"
	}
	return fmt.Sprintf("Page %d of %d", s.pos+1, len(s.queue))
}

// statusBar is the bottom row: facts on the left, key hints on the right.
// Hints are dropped from the end when the terminal is too narrow.
func (a App) statusBar(w int) string {
	r := a.st.chrome
	sep := r.label.Render("  ·  ")
	facts := []string{r.accent.Render(a.profile.Level.String()), r.value.Render(fmt.Sprintf("%d due", a.review.remaining()))}
	if s := a.review; s.correct+s.failed > 0 {
		facts = append(facts, r.success.Render(fmt.Sprintf("✓ %d", s.correct))+r.base.Render(" ")+
			r.danger.Render(fmt.Sprintf("✗ %d", s.failed)))
	}
	if n := a.streak().Current; n > 0 {
		facts = append(facts, r.warning.Render(fmt.Sprintf("%d-day streak", n)))
	}
	left := r.base.Render(" ") + strings.Join(facts, sep)

	hints := a.hints()
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
		return withListen(a.reviewHints(), a.listenHints())
	case Learn:
		return withListen(a.learnHints(), a.listenHints())
	case Write:
		return withListen(a.writeHints(), a.listenHints())
	case Talk:
		return a.talkHints()
	case Fluency:
		return a.fluencyHints()
	case Quiz:
		return withListen(a.quizHints(), a.listenHints())
	case Listen:
		if a.long.on {
			return a.longHints() // its own p says the selected sentence
		}
		return a.dictationHints() // its own ctrl+p and ctrl+s say the sentence
	case Pairs:
		return a.pairsHints() // its own p and s say the word
	case Speak:
		return a.speakHints() // its own p and s say the sentence
	case Level:
		return a.levelHints()
	case Words:
		if a.words.filtering {
			return []string{"type to filter", "enter keep", "esc clear"}
		}
		return []string{"/ filter", "↑↓ scroll", "esc home", "? help"}
	default:
		return []string{"tab screens", ": palette", "esc home", "? help"}
	}
}

// withListen puts the audio hints after the screen's own keys, before the
// general ones (tab, esc, ?), which are the first to drop when narrow.
func withListen(hints, listen []string) []string {
	if len(listen) == 0 {
		return hints
	}
	i := slices.IndexFunc(hints, func(h string) bool {
		return h == "tab screens" || h == "esc home" || h == "? help"
	})
	if i < 0 {
		i = len(hints)
	}
	return slices.Insert(slices.Clone(hints), i, listen...)
}
