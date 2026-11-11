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
	maxTricky = 3 // words listed under "Watch these"
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
	date        domain.Date // the study day the grade was recorded on
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
		card := domain.TrackKnown(before, srs.Review(before, ans.Grade(), a.now()), a.today())
		if before.Direction == domain.Production && s.result.Kind != check.Exact {
			card.LastWrong = strings.TrimSpace(s.input)
		}
		if err := a.save(card); err != nil {
			a.err = err
			return nil
		}
		a.cards[idx] = card
		a.recordGrade(!failed)
		if a.err != nil {
			return nil
		}
	}

	step := &undoStep{pos: s.pos, before: before, failed: failed, date: a.today()}
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
		card := domain.TrackKnown(s.undo.before, srs.Review(s.undo.before, ans.Grade(), a.now()), a.today())
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
	a.recordUndo(u.date, u.failed)
	if a.err != nil {
		return nil
	}
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
		return a.summaryView(w)
	}

	c := a.cards[s.queue[s.pos]]
	title := "Recognize · English → Spanish"
	if c.Direction == domain.Production {
		title = "Produce · Spanish → English"
	}
	if s.practice {
		title = "Mistakes round · " + title
	}

	sections := []string{a.st.main.label.Render(title)}
	if c.Direction == domain.Production {
		sections = append(sections, a.productionView(c, w)...)
	} else {
		sections = append(sections, a.recognitionView(c, w)...)
	}
	if a.notes {
		sections = append(sections, a.marginNotes(w))
	}
	return a.joinSections(w, sections)
}

// joinSections stacks the parts of a page with a ruled line between them,
// like the lines of a notebook. The first part is the page title.
func (a App) joinSections(w int, sections []string) string {
	rule := "\n" + a.st.main.rule.Render(strings.Repeat("─", w)) + "\n"
	return sections[0] + "\n\n" + strings.Join(sections[1:], rule)
}

// headword is the word on its highlight marker, with the pronunciation after
// it when there is room.
func (a App) headword(text string, style lipgloss.Style, extra string, w int) string {
	text = ansi.Truncate(text, max(w-2, 1), "…")
	line := style.Render(" " + text + " ")
	if extra != "" && ansi.StringWidth(line)+3+ansi.StringWidth(extra) <= w {
		line += "   " + a.st.main.meta.Render(extra)
	}
	return line
}

// labeled is a section: a small label and its text indented under it.
// The text wraps to the page; style paints each wrapped line.
func (a App) labeled(label, text string, w int, style func(...string) string) string {
	lines := strings.Split(ansi.Wordwrap(text, max(w-2, 1), ""), "\n")
	for i, l := range lines {
		lines[i] = "  " + style(l)
	}
	return a.st.main.secondary.Render(label) + "\n" + strings.Join(lines, "\n")
}

func (a App) recognitionView(c domain.Card, w int) []string {
	r := a.st.main
	head := a.headword(c.Word, r.word, c.Pronunciation, w)
	if a.current().phase != revealed {
		return []string{head, r.secondary.Render("What does it mean? Say it in your head.") + "\n\n" +
			"  " + r.accent.Render("space") + r.label.Render(" show the answer")}
	}
	return []string{
		head,
		a.labeled("In context", `"`+c.Example+`"`, w, func(l ...string) string { return a.markWord(strings.Join(l, ""), c.Word) }),
		a.labeled("Meaning", c.Meaning, w, r.value.Render),
		r.secondary.Render("How well did you remember it?") + "\n\n  " +
			a.gradeRow(domain.Again, domain.Hard, domain.Good, domain.Easy),
	}
}

// markWord underlines the first appearance of word in line, ignoring case,
// so the learner sees the word inside a real sentence.
func (a App) markWord(line, word string) string {
	r := a.st.main
	i := strings.Index(strings.ToLower(line), strings.ToLower(word))
	if word == "" || i < 0 {
		return r.value.Render(line)
	}
	j := i + len(word)
	return r.value.Render(line[:i]) + r.accent.Underline(true).Render(line[i:j]) + r.value.Render(line[j:])
}

// gradeRow is "1 again   2 hard   3 good   4 easy" for the given answers.
func (a App) gradeRow(answers ...domain.Answer) string {
	r := a.st.main
	styles := map[domain.Answer]lipgloss.Style{
		domain.Again: r.danger, domain.Hard: r.warning, domain.Good: r.info, domain.Easy: r.success,
	}
	parts := make([]string, len(answers))
	for i, ans := range answers {
		parts[i] = styles[ans].Bold(true).Render(fmt.Sprint(int(ans))) + " " + ans.String()
	}
	return strings.Join(parts, "      ")
}

func (a App) productionView(c domain.Card, w int) []string {
	r := a.st.main
	s := a.current()

	// Spanish is never bold: bold is reserved for the English being learned.
	sections := []string{a.headword(c.Meaning, r.prompt, "", w)}
	if gapped, ok := check.Gap(c.Example, c.Word); ok {
		sections = append(sections, a.labeled("In context", `"`+gapped+`"`, w, r.value.Render))
	}

	answer := r.secondary.Render("Your answer") + "\n  " + r.accent.Render("› ") + s.input
	if s.phase == typing {
		answer += r.accent.Render("▏")
	}
	if s.phase != checked {
		return append(sections, answer)
	}

	question := r.secondary.Render("How well did you remember it?") + "\n\n  "
	switch s.result.Kind {
	case check.Exact:
		answer += "\n  " + r.success.Render("✓ correct") + r.label.Render(" · graded good")
		return append(sections, answer,
			"  "+r.accent.Render("enter")+" next      "+r.success.Bold(true).Render("4")+" easy")
	case check.Almost:
		n := s.result.Distance
		answer += "\n  " + a.diffView(s.result.Diff) + "   " +
			r.warning.Render(fmt.Sprintf("✗ almost · %d %s", n, plural(n, "letter", "letters")))
		return append(sections, answer, question+a.gradeRow(domain.Again, domain.Hard))
	default:
		answer += "\n  " + r.danger.Render("✗ not quite") + r.label.Render(" · the answer is ") + r.target.Render(c.Word)
		return append(sections, answer, question+a.gradeRow(domain.Again))
	}
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

// summaryView is the session end as a page to reread: results, the words to
// watch, the next review and one preselected action.
func (a App) summaryView(w int) string {
	r := a.st.main
	s := a.review
	n := len(s.queue)
	minutes := max(int(math.Ceil(s.elapsed.Minutes())), 1)

	top := r.target.Render("Done for today.") + "  " +
		r.secondary.Render(fmt.Sprintf("%d %s in %d min.", n, plural(n, "word", "words"), minutes)) +
		"      " + a.results(s.correct, s.failed)
	if a.round.queue != nil && a.round.done() {
		top += "\n" + r.label.Render("Mistakes round") + "   " + a.results(a.round.correct, a.round.failed)
	}

	sections := []string{top}
	if len(s.tricky) > 0 {
		lines := []string{r.secondary.Render("Watch these")}
		for _, idx := range s.tricky[:min(len(s.tricky), maxTricky)] {
			c := a.cards[idx]
			line := "  " + a.headword(c.Word, r.word, "", w-2)
			if c.LastWrong != "" {
				line += "   " + r.label.Render(fmt.Sprintf("you wrote %q", c.LastWrong))
			}
			lines = append(lines, line)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	sections = append(sections, r.secondary.Render("Next page")+"      "+a.nextReview())

	home := r.accent.Render("enter") + r.label.Render(" back home")
	if k := len(s.tricky); k > 0 {
		sections = append(sections, "  "+r.accent.Bold(true).Render("r")+" "+
			r.target.Render(fmt.Sprintf("practice the %d %s again", k, plural(k, "mistake", "mistakes")))+"        "+home)
	} else {
		sections = append(sections, "  "+home)
	}

	// The summary has no title of its own: "Done for today." is it.
	rule := "\n" + r.rule.Render(strings.Repeat("─", w)) + "\n"
	return strings.Join(sections, rule)
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
