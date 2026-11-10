package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func (a App) fluencyView(w, h int) string {
	width := max(w, 20)
	switch a.fluency.phase {
	case fluencyOwnTopic:
		return a.ownTopicView()
	case fluencyReady:
		return a.readyView(width)
	case fluencyRunning:
		return a.runningView(width, h)
	case fluencyResult:
		return a.fluencyResultView(width)
	}
	return a.topicPickerView(width)
}

// topicPickerView lists the topics and the way to type one's own.
func (a App) topicPickerView(width int) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render("Fluency") + "\n\n")
	b.WriteString(r.target.Render(ansi.Wordwrap("Tell the same story three times, in 4, 3 and 2 minutes. "+
		"Each time, try to say it faster.", width, "")) + "\n")
	b.WriteString(r.secondary.Render("Pick a topic:") + "\n\n")
	for i, topic := range slices.Concat(fluencyTopics, []string{ownTopic}) {
		if i == a.fluency.cursor {
			b.WriteString(r.accent.Render("▸ "+topic) + "\n")
		} else {
			b.WriteString("  " + r.value.Render(topic) + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (a App) ownTopicView() string {
	r := a.st.main
	return r.title.Render("Fluency · Your own topic") + "\n\n" +
		r.secondary.Render("What is your story about?") + "\n\n" +
		r.secondary.Render("› ") + a.fluency.input + r.accent.Render("▏")
}

// readyView is the page before each round: the rounds told so far and
// what the next one asks for.
func (a App) readyView(width int) string {
	r := a.st.main
	f := a.fluency
	var b strings.Builder
	b.WriteString(r.title.Render("Fluency · "+f.topic) + "\n\n")
	if f.round > 0 {
		b.WriteString(a.roundLines(f.round) + "\n\n")
	}
	limit := clockText(roundLimits[f.round])
	b.WriteString(r.accent.Render(fmt.Sprintf("Round %d of 3 · %s", f.round+1, limit)) + "\n")
	tip := "Write the story in English, as much as you can in " + limit +
		". Do not stop to fix mistakes."
	if f.round > 0 {
		tip = "Tell the same story again, now in " + limit + ". Try to say all of it, faster."
	}
	b.WriteString(r.secondary.Render(ansi.Wordwrap(tip, width, "")) + "\n\n")
	b.WriteString(r.label.Render("Press enter to start the clock."))
	return b.String()
}

// runningView is a round in progress: the countdown, the word count and
// as much of the text as fits.
func (a App) runningView(width, h int) string {
	r := a.st.main
	f := a.fluency
	text := f.rounds[f.round].text
	head := r.title.Render(fmt.Sprintf("Fluency · Round %d of 3", f.round+1)) + "\n" +
		r.target.Render(f.topic) + "\n" +
		r.accent.Render(clockText(a.remaining())+" left") +
		r.label.Render(fmt.Sprintf(" · %d words", countWords(text))) + "\n\n"

	lines := strings.Split(ansi.Wordwrap(text+"▏", width, ""), "\n")
	room := max(h-strings.Count(head, "\n")-1, 1)
	lines = lines[max(len(lines)-room, 0):]
	return head + strings.Join(lines, "\n")
}

// fluencyResultView shows the words per minute of each round, the change
// from the first to the last and the start of each text.
func (a App) fluencyResultView(width int) string {
	r := a.st.main
	f := a.fluency
	var b strings.Builder
	b.WriteString(r.title.Render("Fluency · "+f.topic) + "\n\n")
	b.WriteString(a.roundLines(len(roundLimits)) + "\n\n")

	first := wordsPerMinute(f.rounds[0].text, f.rounds[0].elapsed, roundLimits[0])
	last := wordsPerMinute(f.rounds[2].text, f.rounds[2].elapsed, roundLimits[2])
	change := fmt.Sprintf("Round 1 → 3: %+.1f wpm", last-first)
	if last > first {
		b.WriteString(r.success.Render("✓ "+change) + "\n\n")
	} else {
		b.WriteString(r.secondary.Render(change) + "\n\n")
	}

	b.WriteString(r.secondary.Render("Your texts") + "\n")
	for i, round := range f.rounds {
		one := strings.Join(strings.Fields(round.text), " ")
		if one == "" {
			one = "(nothing written)"
		}
		b.WriteString(r.label.Render(fmt.Sprintf("  %d  ", i+1)) + ansi.Truncate(one, max(width-5, 1), "…") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// roundLines is one line per finished round: its limit, words and pace.
func (a App) roundLines(n int) string {
	r := a.st.main
	lines := make([]string, n)
	for i := range n {
		round := a.fluency.rounds[i]
		lines[i] = r.value.Render(fmt.Sprintf("Round %d · %s · %d words · %.1f wpm", i+1,
			clockText(roundLimits[i]), countWords(round.text),
			wordsPerMinute(round.text, round.elapsed, roundLimits[i])))
	}
	return strings.Join(lines, "\n")
}

// clockText writes d as minutes and seconds, like 3:05.
func clockText(d time.Duration) string {
	s := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a App) fluencyHints() []string {
	switch a.fluency.phase {
	case fluencyPick:
		return []string{"↑↓ choose", "enter pick", "tab screens", "esc home"}
	case fluencyOwnTopic:
		return []string{"type a topic", "enter ok", "esc back"}
	case fluencyReady:
		return []string{"enter start", "n new topic", "tab screens", "esc home"}
	case fluencyRunning:
		return []string{"type your story", "ctrl+d end round", "esc home"}
	}
	return []string{"n new topic", "tab screens", "esc home"}
}
