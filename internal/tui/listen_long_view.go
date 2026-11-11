package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

// longTitle is the title of every long-listening page.
const longTitle = "Listen · Long listening"

// longView is the long-listening half of Listen: why it cannot start, the
// progress while listening, the questions, then the transcript.
func (a App) longView(w, h int) string {
	r := a.st.main
	s := a.long
	width := max(w, 20)
	var b strings.Builder
	b.WriteString(r.title.Render(longTitle) + "\n\n")
	switch {
	case a.speaker == nil:
		b.WriteString(r.warning.Render(audioOffNotice) + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap("Long listening needs a voice: run `teacher setup voice`, then `teacher doctor`.", width, "")))
		return b.String()
	case a.teacher == nil:
		b.WriteString(r.secondary.Render(ansi.Wordwrap("The AI teacher is off. Install Gentle Shell (the pi command) and see `teacher models`.", width, "")))
		return b.String()
	}
	switch s.phase {
	case longLoading:
		b.WriteString(r.secondary.Render("Writing a text for you… this takes about 30 seconds."))
	case longFailed:
		b.WriteString(r.danger.Render(ansi.Wordwrap("✗ Could not get a text: "+s.err.Error(), width, "")) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
	case longListening:
		b.WriteString(a.longProgress(width))
	case longQuestions:
		b.WriteString(a.longQuestion(width))
	case longTranscript:
		b.WriteString(a.longTranscript(width, max(h-2, 3)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// longKind names what the learner hears, without giving any of it away.
func (a App) longKind() string {
	if a.long.text.Kind == teacher.ListeningStory {
		return "A story"
	}
	return "A dialogue"
}

// longProgress is the listening page: what it is and how far it has
// played, but never its words.
func (a App) longProgress(width int) string {
	r := a.st.main
	s := a.long
	n := len(s.text.Lines)
	var b strings.Builder
	b.WriteString(r.value.Render(a.longKind()+" · "+s.text.Title) + "\n\n")
	switch {
	case s.playing:
		b.WriteString(r.accent.Render("▶ playing") + r.label.Render(fmt.Sprintf(" · Sentence %d of %d", s.pos+1, n)))
	case s.heard:
		b.WriteString(r.success.Render("✓ heard to the end") + r.label.Render(fmt.Sprintf(" · %d sentences", n)))
	default:
		b.WriteString(r.secondary.Render(fmt.Sprintf("Paused at sentence %d of %d", s.pos+1, n)))
	}
	bar := max(min(width-2, 40), 10)
	done := bar * (s.pos + 1) / max(n, 1)
	if s.heard && !s.playing {
		done = bar
	}
	b.WriteString("\n" + r.accent.Render(strings.Repeat("█", done)) + r.label.Render(strings.Repeat("░", bar-done)) + "\n\n")
	b.WriteString(r.label.Render(ansi.Wordwrap("Listen first: the questions come next, and the text after them.", width, "")))
	if s.heard {
		b.WriteString("\n\n" + r.label.Render("Press enter for the questions."))
	}
	return b.String()
}

// longQuestion is the question on screen with its options and, once
// answered, whether the learner was right.
func (a App) longQuestion(width int) string {
	r := a.st.main
	s := a.long
	q := s.text.Questions[s.question]
	var b strings.Builder
	b.WriteString(r.label.Render(fmt.Sprintf("Question %d of %d · ✓ %d", s.question+1, len(s.text.Questions), s.score())) + "\n\n")
	b.WriteString(r.target.Render(ansi.Wordwrap(q.Question, width, "")) + "\n\n")
	for i, o := range q.Options {
		line := fmt.Sprintf("%d  %s", i+1, o)
		switch {
		case !s.answered():
			b.WriteString(r.value.Render(line) + "\n")
		case i == q.Answer:
			b.WriteString(r.success.Render(line) + "\n")
		case i == s.answers[s.question]:
			b.WriteString(r.danger.Render(line) + "\n")
		default:
			b.WriteString(r.label.Render(line) + "\n")
		}
	}
	if !s.answered() {
		return b.String()
	}
	b.WriteString("\n")
	if s.answers[s.question] == q.Answer {
		b.WriteString(r.success.Render("✓ Right: "+q.Right()) + "\n")
	} else {
		b.WriteString(r.danger.Render(fmt.Sprintf("✗ Not quite. The answer is %d: %s", q.Answer+1, q.Right())) + "\n")
	}
	if q.Why != "" {
		b.WriteString(r.secondary.Render(ansi.Wordwrap(q.Why, width, "")) + "\n")
	}
	next := "Press enter for the next question."
	if s.question == len(s.text.Questions)-1 {
		next = "Saved in today's log. Press enter for the transcript."
	}
	return b.String() + "\n" + r.label.Render(next)
}

// longTranscript is the score and the sentences that fit in rows lines,
// around the selected one, each with its speaker.
func (a App) longTranscript(width, rows int) string {
	r := a.st.main
	s := a.long
	var b strings.Builder
	score := fmt.Sprintf("%d/%d", s.score(), len(s.text.Questions))
	b.WriteString(r.value.Render(s.text.Title) + r.label.Render(" · questions ") + r.accent.Render(score) + "\n\n")
	rows = max(rows-2, 1)
	n := len(s.text.Lines)
	first := max(min(s.pos-rows/2, n-rows), 0)
	for i := first; i < min(first+rows, n); i++ {
		l := s.text.Lines[i]
		pointer := "  "
		if i == s.pos {
			pointer = r.accent.Render("▸ ")
		}
		line := ansi.Truncate(l.Speaker+": "+l.Text, max(width-2, 1), "…")
		if i == s.pos {
			b.WriteString(pointer + r.target.Render(line) + "\n")
		} else {
			b.WriteString(pointer + r.value.Render(line) + "\n")
		}
	}
	return b.String()
}

// longHints are the keys of the long listening.
func (a App) longHints() []string {
	s := a.long
	if a.speaker == nil || a.teacher == nil {
		return []string{"s dictation", "tab screens", "esc home", "? help"}
	}
	switch s.phase {
	case longFailed:
		return []string{"enter try again", "s dictation", "esc home"}
	case longListening:
		pause := "space pause"
		if !s.playing {
			pause = "space play"
		}
		hints := []string{pause, "r replay all"}
		if s.heard {
			hints = append(hints, "enter questions")
		}
		return append(hints, "s dictation", "esc home")
	case longQuestions:
		if s.answered() {
			return []string{"enter next", "esc home"}
		}
		return []string{"1–3 answer", "esc home"}
	case longTranscript:
		return []string{"↑↓ move", "p hear it", "n new text", "s dictation", "esc home"}
	}
	return []string{"s dictation", "tab screens", "esc home"}
}
