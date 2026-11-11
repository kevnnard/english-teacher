package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

func (a App) talkView(w, h int) string {
	r := a.st.main
	t := a.talk
	width := max(w, 20)
	switch {
	case a.teacher == nil:
		return r.title.Render("Talk") + "\n\n" +
			r.secondary.Render("The AI teacher is off. Install Gentle Shell (the pi command) and see `teacher models`.")
	case t.phase == talkPick:
		return a.pickerView(width)
	case t.phase == talkSummary:
		return a.talkSummaryView(width)
	}
	return a.chatView(width, h)
}

// pickerView lists the scenarios, with the goal of the one under the pointer.
func (a App) pickerView(width int) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render("Talk") + "\n\n")
	b.WriteString(r.target.Render("Practice a short conversation. The teacher plays the other person.") + "\n")
	b.WriteString(r.secondary.Render("Pick a situation:") + "\n\n")
	for i, sc := range teacher.Scenarios {
		if i == a.talk.cursor {
			b.WriteString(r.accent.Render("▸ "+sc.Title) + "\n")
		} else {
			b.WriteString("  " + r.value.Render(sc.Title) + "\n")
		}
	}
	goal := teacher.Scenarios[a.talk.cursor].Goal
	b.WriteString("\n" + r.label.Render(ansi.Wordwrap("Goal: "+goal, width, "")))
	return b.String()
}

// chatView is the role-play: the goal and frames on top, then as much of
// the transcript as fits, then the input or what the app is waiting for.
func (a App) chatView(width, h int) string {
	r := a.st.main
	t := a.talk
	var head strings.Builder
	head.WriteString(r.title.Render("Talk · "+t.scenario.Title) + "\n")
	head.WriteString(r.target.Render(ansi.Wordwrap("Goal: "+t.scenario.Goal, width, "")) + "\n")
	if frames := t.scenario.FramesFor(a.profile.Level); len(frames) > 0 {
		head.WriteString(r.secondary.Render("Frames") + "\n")
		for _, f := range frames {
			head.WriteString("  " + r.accent.Render("• "+f) + "\n")
		}
	}
	turn := min(t.learnerTurns()+1, maxTalkTurns)
	head.WriteString(r.label.Render(fmt.Sprintf("Turn %d/%d", turn, maxTalkTurns)) + "\n")

	var chat []string
	for _, l := range t.lines {
		chat = append(chat, a.chatLine(l, width)...)
	}
	var foot string
	switch t.phase {
	case talkWaiting:
		chat = append(chat, a.chatLine(teacher.Line{Text: t.pending}, width)...)
		foot = r.secondary.Render(t.scenario.Partner + " is answering…")
	case talkSummarizing:
		foot = r.secondary.Render("Writing your feedback… this takes about 10 seconds.")
	case talkSummaryFail:
		foot = r.danger.Render(ansi.Wordwrap("✗ Could not get your feedback: "+t.err.Error(), width, "")) + "\n" +
			r.label.Render("Press enter to try again.")
	case talkRecording:
		foot = r.danger.Render("● Recording… ctrl+r to stop") + r.label.Render(" · esc cancel")
	case talkListening:
		foot = r.accent.Render("Listening…")
	default:
		if t.err != nil {
			foot = r.danger.Render(ansi.Wordwrap("✗ Could not get an answer: "+t.err.Error(), width, "")) + "\n" +
				r.label.Render("Your line is still here; press enter to try again.") + "\n"
		}
		foot += r.secondary.Render("› ") + t.input + r.accent.Render("▏")
	}

	if a.listen.notice != "" {
		foot += "\n" // the notice takes the last row; keep the input above it
	}

	// Keep the newest lines when the transcript is taller than the page.
	room := max(h-strings.Count(head.String(), "\n")-strings.Count(foot, "\n")-3, 1)
	chat = chat[max(len(chat)-room, 0):]
	return head.String() + "\n" + strings.Join(chat, "\n") + "\n\n" + foot
}

// chatLine renders one transcript line, wrapped, and a learner line's
// recast under it in a quiet color, so it helps without interrupting.
func (a App) chatLine(l teacher.Line, width int) []string {
	r := a.st.main
	who, style := "You", r.value
	if l.Tutor {
		who, style = a.talk.scenario.Partner, r.target
	}
	text := ansi.Wordwrap(who+": "+l.Text, width, "")
	lines := strings.Split(style.Render(text), "\n")
	if l.Recast != "" {
		recast := ansi.Wordwrap("↳ "+l.Recast, width-2, "")
		lines = append(lines, strings.Split(indent(r.label.Render(recast), 2), "\n")...)
	}
	return lines
}

// talkSummaryView is the end of a role-play: the teacher's points, the
// errors and the cards they became.
func (a App) talkSummaryView(width int) string {
	r := a.st.main
	t := a.talk
	var b strings.Builder
	b.WriteString(r.title.Render("Talk · "+t.scenario.Title) + "\n")
	b.WriteString(r.label.Render("Feedback") + "\n\n")
	for _, p := range t.feedback.Points {
		b.WriteString(indent(r.target.Render(ansi.Wordwrap("• "+p, width-2, "")), 2) + "\n")
	}

	majors := t.feedback.Majors()
	if len(majors) > 0 {
		b.WriteString("\n" + r.secondary.Render("To practice") + "\n")
		for _, f := range majors {
			b.WriteString("  " + r.danger.Render("✗ "+f.Wrong) + r.label.Render(" → ") + r.success.Render(f.Right) + "\n")
			if f.Why != "" {
				b.WriteString(indent(r.secondary.Render(ansi.Wordwrap(f.Why, width-4, "")), 4) + "\n")
			}
		}
	}
	var minors []teacher.Fix
	for _, f := range t.feedback.Fixes {
		if !isMajor(majors, f) {
			minors = append(minors, f)
		}
	}
	if len(minors) > 0 {
		b.WriteString("\n" + r.label.Render("Small details") + "\n")
		for _, f := range minors {
			b.WriteString("  " + r.label.Render(f.Wrong+" → "+f.Right) + "\n")
		}
	}

	b.WriteString("\n" + a.talkCardsLine())
	if t.logErr != nil {
		b.WriteString("\n" + r.danger.Render("✗ Could not save the error log: "+t.logErr.Error()))
	}
	return b.String()
}

// isMajor reports whether f is one of majors.
func isMajor(majors []teacher.Fix, f teacher.Fix) bool {
	for _, m := range majors {
		if m == f {
			return true
		}
	}
	return false
}

// talkCardsLine says which cards the major errors became.
func (a App) talkCardsLine() string {
	r := a.st.main
	t := a.talk
	switch {
	case t.cardErr != nil:
		return r.danger.Render("✗ Could not add the cards: "+t.cardErr.Error()) + "\n" + r.label.Render("Press s to try again.")
	case t.added == 1:
		return r.success.Render("✓ ") + r.secondary.Render("Added 1 card to practice.")
	case t.added > 1:
		return r.success.Render("✓ ") + r.secondary.Render(fmt.Sprintf("Added %d cards to practice.", t.added))
	case len(t.feedback.Majors()) == 0:
		return r.success.Render("✓ ") + r.secondary.Render("No major errors, so no new cards.")
	}
	return r.secondary.Render("These errors are already in your cards.")
}

func (a App) talkHints() []string {
	if a.teacher == nil {
		return []string{"tab screens", "esc home", "? help"}
	}
	t := a.talk
	switch t.phase {
	case talkPick:
		return []string{"↑↓ choose", "enter start", "tab screens", "esc home"}
	case talkChatting:
		hints := append([]string{"type your line", "enter send"}, a.talkVoiceHints()...)
		if t.learnerTurns() > 0 {
			hints = append(hints, "ctrl+d finish")
		}
		return append(hints, "esc home")
	case talkRecording:
		return []string{"ctrl+r stop", "esc cancel"}
	case talkListening:
		return []string{"esc cancel"}
	case talkSummaryFail:
		return []string{"enter try again", "esc home"}
	case talkSummary:
		var hints []string
		if len(t.cards) > 0 {
			hints = append(hints, "s save cards")
		}
		return append(hints, "n new role-play", "tab screens", "esc home")
	}
	return []string{"tab screens", "esc home"}
}
