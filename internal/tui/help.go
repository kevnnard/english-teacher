package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

// helpSection is one titled group of keys in the `?` overlay.
type helpSection struct {
	title string
	keys  [][2]string
}

// helpSections is the text of the `?` overlay: the first section applies
// everywhere and each other one to the screen of the same name. The README
// keys table mirrors it; change both together.
var helpSections = []helpSection{
	{"Everywhere", [][2]string{
		{"tab / shift+tab", "next / previous screen"},
		{":", "command palette: type a screen name, enter to go"},
		{"?", "show / hide this help"},
		{"c", "show / hide the margin notes during review"},
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
		{"u", "undo the last grade (ctrl+z while typing an answer)"},
		{"r", "review the session's mistakes again (dates don't move)"},
		{"p", "listen to the word (typed: once checked); p p slower"},
	}},
	{"Learn", [][2]string{
		{"enter", "get a short text written for you"},
		{"↑ ↓ / j k", "move through the glossary"},
		{"space", "mark or unmark the word to study"},
		{"a", "done: add the marked words as cards"},
		{"n", "a new text"},
		{"p / P", "hear the selected word / the whole text; p p slower"},
	}},
	{"Write", [][2]string{
		{"enter", "get a task; while writing, a new line"},
		{"ctrl+d", "send your text to the teacher"},
		{"enter", "fixing or rewriting: submit (empty: I don't know)"},
		{"d", "show / hide the minor details in the feedback"},
		{"s", "save the error cards again if saving failed"},
		{"n", "a new task from the feedback or the result"},
		{"p", "feedback, checked rewrite: hear the right version"},
		{"esc", "back to Home; your work stays"},
	}},
	{"Talk", [][2]string{
		{"↑ ↓ / j k", "choose a situation"},
		{"enter", "start it; send your line (again after a failed answer)"},
		{"ctrl+r", "say your line; ctrl+r stops, esc cancels; then edit"},
		{"ctrl+p", "hear the last reply again; twice quickly: slower"},
		{"ctrl+t", "voice mode on / off: replies read aloud"},
		{"ctrl+d", "finish the role-play early and get feedback"},
		{"s", "save the error cards again if saving failed"},
		{"n", "a new role-play from the feedback"},
		{"esc", "back to Home; the conversation stays"},
	}},
	{"Fluency", [][2]string{
		{"↑ ↓ / j k", "choose a topic (the last one: type your own)"},
		{"enter", "pick it; then start each round's clock"},
		{"ctrl+d", "end the round early; the time stops"},
		{"n", "a new topic between rounds or from the result"},
		{"esc", "back to Home; the clock keeps running"},
	}},
	{"Quiz", [][2]string{
		{"↑ ↓ / j k", "choose a vocabulary or a grammar quiz"},
		{"enter", "start it; after the feedback, the next question"},
		{"1–4", "choose an option"},
		{"enter", "check a typed answer (empty: I don't know)"},
		{"enter", "after a failed grammar quiz: try again"},
		{"n", "a new quiz from the result"},
		{"p", "after an answer: hear the right one; p p slower"},
		{"esc", "back to Home; the quiz stays"},
	}},
	{"Listen", [][2]string{
		{"enter", "start a dictation; after the feedback, the next one"},
		{"ctrl+p", "hear the sentence again (p is a letter here)"},
		{"ctrl+s", "hear it slower"},
		{"enter", "check what you typed (empty: I don't know)"},
		{"n", "a new dictation from the result"},
		{"esc", "back to Home; the dictation stays"},
	}},
	{"Pairs", [][2]string{
		{"enter", "start a round; after the feedback, the next word"},
		{"1 / 2", "choose which word you heard; after it, hear each one"},
		{"p / s", "hear the word again / slower"},
		{"n", "a new round from the result"},
		{"esc", "back to Home; the round stays (saved only at the end)"},
	}},
	{"Speak", [][2]string{
		{"enter", "start a round; start / stop recording; the next one"},
		{"p / s", "hear the sentence again / slower"},
		{"esc", "while recording: cancel it; else back to Home"},
		{"r", "after the marks: try the same sentence again"},
		{"n", "a new round from the result"},
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
	// Only the screen's own keys fit next to the general ones on 24 rows.
	sections := []helpSection{helpSections[0]}
	for _, sec := range helpSections[1:] {
		if sec.title == a.screen.String() {
			sections = append(sections, sec)
		}
	}
	for _, sec := range sections {
		b.WriteString("\n" + r.label.Render(sec.title) + "\n")
		for _, k := range sec.keys {
			fmt.Fprintf(&b, "  %s %s\n", r.accent.Render(fmt.Sprintf("%-16s", k[0])), k[1])
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// marginNotes is the learner's own history with the card on screen, written
// at the foot of the page. Tips and "use it today" need the LLM and arrive
// in M3, so they are not here.
func (a App) marginNotes(w int) string {
	r := a.st.main
	s := a.current()
	c := a.cards[s.queue[s.pos]]
	if s.phase == revealed || s.phase == checked {
		// Show the history the card had when it came up, not after this grade.
		if s.undo != nil && s.undo.pos == s.pos {
			c = s.undo.before
		}
	}

	history := "new word"
	if seen := c.TimesCorrect + c.TimesFailed; seen > 0 {
		history = fmt.Sprintf("seen %d · ✓ %d · ✗ %d", seen, c.TimesCorrect, c.TimesFailed)
	}
	next := srs.Review(c, domain.Good.Grade(), a.now()).IntervalDays
	when := "tomorrow"
	if next > 1 {
		when = fmt.Sprintf("in %d days", next)
	}

	lines := []string{
		r.meta.Render("Margin notes"),
		"  " + r.label.Render(ansi.Truncate(history+" · next if good: "+when, max(w-2, 1), "…")),
	}
	if c.LastWrong != "" {
		lines = append(lines, "  "+r.label.Render(ansi.Truncate(fmt.Sprintf("last: wrote %q", c.LastWrong), max(w-2, 1), "…")))
	}
	return strings.Join(lines, "\n")
}
