package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

const (
	cardWidth = 52 // the widest the review card gets, borders included
	maxTricky = 3  // words listed under "Tricky today"
)

// phase is where the learner is with the card on screen.
type phase int

const (
	front    phase = iota // recognition: answer hidden
	revealed              // recognition: answer visible, waiting for 1–4
	typing                // production: typing the English
	checked               // production: answer checked, waiting for a grade or enter
)

// session is one review sitting. The same type runs the "mistakes round",
// with practice set: then nothing is saved and SM-2 dates do not move.
type session struct {
	queue    []int // indices into App.cards, in review order
	pos      int   // how many cards of queue are done
	practice bool

	phase  phase
	input  string       // the typed answer in production
	result check.Result // the verdict once checked

	correct, failed int
	tricky          []int // indices of cards failed this session, oldest first

	undo    *undoStep // the last grade, until the next card is graded
	started time.Time
	elapsed time.Duration // set when the last card is graded
}

// undoStep remembers enough to take a grade back.
type undoStep struct {
	pos         int         // queue position of the graded card
	before      domain.Card // the card as it was before the grade
	failed      bool
	addedTricky bool
}

func newSession(cards []domain.Card, queue []int, practice bool, now time.Time) session {
	s := session{queue: queue, practice: practice, started: now}
	s.resetCard(cards)
	return s
}

func (s session) remaining() int { return len(s.queue) - s.pos }

func (s session) done() bool { return s.pos >= len(s.queue) }

// resetCard puts the card at pos in its first phase.
func (s *session) resetCard(cards []domain.Card) {
	s.input, s.result, s.phase = "", check.Result{}, front
	if !s.done() && cards[s.queue[s.pos]].Direction == domain.Production {
		s.phase = typing
	}
}

// current is the session the learner is working through.
func (a *App) current() *session {
	if a.inRound {
		return &a.round
	}
	return &a.review
}

func (a App) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := a.current()
	k := msg.String()

	// u is a letter while typing an answer, so ctrl+z undoes in every phase.
	if (k == "ctrl+z" || k == "u" && s.phase != typing) && s.undo != nil {
		return a, a.undo()
	}

	switch {
	case s.done():
		switch {
		case k == "enter":
			a.screen = Home
		case k == "r" && len(a.review.tricky) > 0:
			a.round = newSession(a.cards, a.review.tricky, true, a.now())
			a.inRound = true
		}
	case s.phase == front:
		if k == "space" {
			s.phase = revealed
		}
	case s.phase == revealed:
		if ans, ok := answerKey(k); ok {
			return a, a.grade(ans, true)
		}
	case s.phase == typing:
		switch k {
		case "enter":
			return a, a.checkAnswer()
		case "esc":
			a.screen = Home
		case "backspace":
			s.input = dropLastRune(s.input)
		default:
			s.input += msg.Text
		}
	case s.phase == checked:
		return a, a.updateChecked(k)
	}
	return a, nil
}

// answerKey maps the 1–4 keys to answers.
func answerKey(k string) (domain.Answer, bool) {
	if len(k) != 1 {
		return 0, false
	}
	ans, err := domain.NewAnswer(int(k[0] - '0'))
	return ans, err == nil
}

// checkAnswer compares the typed answer with the word. An exact answer is
// graded "good" at once; the learner may still upgrade it to "easy".
func (a *App) checkAnswer() tea.Cmd {
	s := a.current()
	c := a.cards[s.queue[s.pos]]
	s.result = check.Answer(s.input, c.Word)
	s.phase = checked
	if s.result.Kind == check.Exact {
		return a.grade(domain.Good, false)
	}
	return nil
}

// updateChecked handles keys once a production answer has been checked.
// The verdict limits the grades: a near miss is "again" or "hard", a wrong
// answer only "again". Typing never silently counts as correct.
func (a *App) updateChecked(k string) tea.Cmd {
	s := a.current()
	switch s.result.Kind {
	case check.Exact:
		switch k {
		case "enter":
			a.advance()
		case "4":
			return a.regrade(domain.Easy)
		}
	case check.Almost:
		if k == "1" || k == "2" {
			ans, _ := answerKey(k)
			return a.grade(ans, true)
		}
	case check.Wrong:
		if k == "1" {
			return a.grade(domain.Again, true)
		}
	}
	return nil
}

// grade records ans for the card on screen and, if next is set, moves on.
//
// Saving synchronously is fine for a local SQLite file and guarantees
// nothing is lost if the learner quits right after answering.
func (a *App) grade(ans domain.Answer, next bool) tea.Cmd {
	s := a.current()
	idx := s.queue[s.pos]
	before := a.cards[idx]
	failed := !ans.Grade().Passed()

	if !s.practice {
		card := srs.Review(before, ans.Grade(), a.now())
		if before.Direction == domain.Production && s.result.Kind != check.Exact {
			card.LastWrong = strings.TrimSpace(s.input)
		}
		if err := a.save(card); err != nil {
			a.err = err
			return nil
		}
		a.cards[idx] = card
	}

	step := &undoStep{pos: s.pos, before: before, failed: failed}
	if failed {
		s.failed++
		if !s.practice && !slices.Contains(s.tricky, idx) {
			s.tricky = append(s.tricky, idx)
			step.addedTricky = true
		}
	} else {
		s.correct++
	}
	if !s.practice {
		s.undo = step
	}
	if next {
		a.advance()
	}
	return nil
}

// regrade replaces the automatic "good" of an exact answer with ans.
// Both are passing grades, so the session counts do not change.
func (a *App) regrade(ans domain.Answer) tea.Cmd {
	s := a.current()
	if !s.practice && s.undo != nil {
		card := srs.Review(s.undo.before, ans.Grade(), a.now())
		if err := a.save(card); err != nil {
			a.err = err
			return nil
		}
		a.cards[s.queue[s.pos]] = card
	}
	a.advance()
	return nil
}

// advance shows the next card, or ends the session after the last one.
func (a *App) advance() {
	s := a.current()
	s.pos++
	s.resetCard(a.cards)
	if s.done() {
		s.elapsed = a.now().Sub(s.started)
		if a.inRound {
			a.inRound = false // back to the summary of the real session
		}
	}
}

// undo takes back the last grade: the card is restored in memory and in the
// store, and comes back on screen from its first phase.
func (a *App) undo() tea.Cmd {
	s := a.current()
	u := s.undo
	idx := s.queue[u.pos]
	if err := a.save(u.before); err != nil {
		a.err = err
		return nil
	}
	a.cards[idx] = u.before
	if u.failed {
		s.failed--
	} else {
		s.correct--
	}
	if u.addedTricky {
		s.tricky = s.tricky[:len(s.tricky)-1]
	}
	s.pos, s.undo = u.pos, nil
	s.resetCard(a.cards)
	return nil
}

func (a App) reviewHints() []string {
	s := a.current()
	var hints []string
	switch {
	case len(s.queue) == 0:
		return []string{"esc home", "? help"}
	case s.done():
		if len(a.review.tricky) > 0 {
			hints = append(hints, "r again")
		}
		hints = append(hints, "enter home")
	case s.phase == front:
		hints = append(hints, "space reveal")
	case s.phase == revealed:
		hints = append(hints, "1-4 grade")
	case s.phase == typing:
		return []string{"type the English", "enter check", "esc home"}
	case s.result.Kind == check.Exact:
		hints = append(hints, "enter next", "4 easy")
	case s.result.Kind == check.Almost:
		hints = append(hints, "1 again", "2 hard")
	default:
		hints = append(hints, "1 again")
	}
	if s.undo != nil {
		hints = append(hints, "u undo")
	}
	if !s.done() {
		hints = append(hints, "esc home")
	}
	return append(hints, "? help")
}

func (a App) reviewView(w int) string {
	s := a.current()
	switch {
	case len(a.review.queue) == 0:
		return a.st.main.secondary.Render("No reviews due — come back tomorrow.")
	case s.done():
		return a.summaryView()
	}

	c := a.cards[s.queue[s.pos]]
	width := min(cardWidth, w)
	title := "Recognize · English → Spanish"
	if c.Direction == domain.Production {
		title = "Produce · Spanish → English"
	}
	if s.practice {
		title = "Mistakes round · " + title
	}
	header := a.spread(width, a.st.main.label.Render(title), a.st.main.label.Render(fmt.Sprintf("%d / %d", s.pos+1, len(s.queue))))

	if c.Direction == domain.Production {
		return header + "\n\n" + a.productionView(c, width)
	}
	return header + "\n\n" + a.recognitionView(c, width)
}

// spread puts left and right at the two ends of a line of width w.
func (a App) spread(w int, left, right string) string {
	gap := max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 2)
	return left + strings.Repeat(" ", gap) + right
}

func (a App) recognitionView(c domain.Card, width int) string {
	r := a.st.main
	var back []cardLine
	hint := r.label.Render("space reveal")
	if a.current().phase == revealed {
		back = []cardLine{{c.Meaning, r.value}, {`"` + c.Example + `"`, r.secondary.Italic(true)}}
		hint = a.gradeRow(domain.Again, domain.Hard, domain.Good, domain.Easy)
	}
	return a.cardBox(width, []cardLine{{c.Word, r.target}, {c.Pronunciation, r.meta}}, back) + "\n  " + hint
}

// gradeRow is "1 again   2 hard   3 good   4 easy" for the given answers.
func (a App) gradeRow(answers ...domain.Answer) string {
	r := a.st.main
	styles := map[domain.Answer]lipgloss.Style{
		domain.Again: r.danger, domain.Hard: r.warning, domain.Good: r.accent, domain.Easy: r.success,
	}
	parts := make([]string, len(answers))
	for i, ans := range answers {
		parts[i] = styles[ans].Render(fmt.Sprint(int(ans))) + " " + ans.String()
	}
	return strings.Join(parts, "   ")
}

func (a App) productionView(c domain.Card, width int) string {
	r := a.st.main
	s := a.current()
	center := func(text string, style lipgloss.Style) string {
		text = ansi.Truncate(text, width, "…")
		return strings.Repeat(" ", max((width-ansi.StringWidth(text))/2, 0)) + style.Render(text)
	}

	// Spanish is never bold: bold is reserved for the English being learned.
	lines := []string{"", center(c.Meaning, r.value)}
	if gapped, ok := check.Gap(c.Example, c.Word); ok {
		lines = append(lines, center(`"`+gapped+`"`, r.secondary.Italic(true)))
	}
	lines = append(lines, "")

	input := r.accent.Render("› ") + s.input
	if s.phase == typing {
		input += r.accent.Render("▏")
	}
	lines = append(lines, input, "")

	if s.phase == checked {
		switch s.result.Kind {
		case check.Exact:
			lines = append(lines,
				"  "+r.success.Render("✓ correct")+r.label.Render(" · graded good"),
				"", "  "+r.accent.Render("enter")+" next   "+r.success.Render("4")+" easy")
		case check.Almost:
			n := s.result.Distance
			lines = append(lines,
				"  "+a.diffView(s.result.Diff)+"   "+r.warning.Render(fmt.Sprintf("✗ almost · %d %s", n, plural(n, "letter", "letters"))),
				"", "  "+a.gradeRow(domain.Again, domain.Hard))
		default:
			lines = append(lines,
				"  "+r.danger.Render("✗ not quite")+r.label.Render(" · the answer is ")+r.target.Render(c.Word),
				"", "  "+a.gradeRow(domain.Again))
		}
	}
	return strings.Join(lines, "\n")
}

// diffView colors a letter diff: typed-by-mistake letters struck in red,
// missing letters underlined in green, each change in brackets so the
// difference still reads without color: nice to me[a→e]t you.
func (a App) diffView(diff []check.Op) string {
	r := a.st.main
	var b strings.Builder
	for _, g := range check.Group(diff) {
		if g.Equal != "" {
			b.WriteString(r.value.Render(g.Equal))
			continue
		}
		b.WriteString(r.label.Render("[") + r.deleted.Render(g.Deleted) + r.label.Render("→") +
			r.inserted.Render(g.Inserted) + r.label.Render("]"))
	}
	return b.String()
}

// summaryView is the session end: results, tricky words, the next review
// and one preselected action.
func (a App) summaryView() string {
	r := a.st.main
	s := a.review
	n := len(s.queue)
	minutes := max(int(math.Ceil(s.elapsed.Minutes())), 1)

	var b strings.Builder
	b.WriteString(r.target.Render("Done.") + " " + r.secondary.Render(fmt.Sprintf("%d reviewed in %d min.", n, minutes)) +
		"   " + a.results(s.correct, s.failed) + "\n")
	if a.round.queue != nil && a.round.done() {
		b.WriteString(r.label.Render("Mistakes round") + "   " + a.results(a.round.correct, a.round.failed) + "\n")
	}

	if len(s.tricky) > 0 {
		b.WriteString("\n" + r.label.Render("Tricky today") + "\n")
		for _, idx := range s.tricky[:min(len(s.tricky), maxTricky)] {
			b.WriteString("  " + r.target.Render(a.cards[idx].Word) + "\n")
		}
	}

	b.WriteString("\n" + r.label.Render("Next review") + "   " + a.nextReview() + "\n\n")

	if k := len(s.tricky); k > 0 {
		b.WriteString(r.accent.Render("▸ r ") + r.target.Render(fmt.Sprintf("review the %d %s again", k, plural(k, "mistake", "mistakes"))) + "\n")
		b.WriteString("  " + r.label.Render("enter home"))
	} else {
		b.WriteString(r.accent.Render("▸ enter ") + r.label.Render("home"))
	}
	return b.String()
}

func (a App) results(correct, failed int) string {
	r := a.st.main
	return r.success.Render(fmt.Sprintf("✓ %d", correct)) + "   " + r.danger.Render(fmt.Sprintf("✗ %d", failed))
}

// nextReview is "tomorrow · 5 cards", from the schedules of every card.
func (a App) nextReview() string {
	now := a.now()
	day, n, ok := srs.NextReview(a.cards, now)
	if !ok {
		return "nothing scheduled"
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	var when string
	switch days := int(day.Sub(today).Hours() / 24); days {
	case 0:
		when = "today"
	case 1:
		when = "tomorrow"
	default:
		when = day.Format("Mon 2 Jan")
	}
	return fmt.Sprintf("%s · %d %s", when, n, plural(n, "card", "cards"))
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
