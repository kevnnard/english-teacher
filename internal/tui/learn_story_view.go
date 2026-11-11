package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

// storyView is the story half of Learn: a title, a line with the chapter
// and the words read this week, then the step of the chapter on screen.
func (a App) storyView(w, h int) string {
	r := a.st.main
	s := a.story
	var b strings.Builder
	title := "Story"
	if s.saved.Title != "" {
		title = s.saved.Title
	}
	b.WriteString(r.title.Render(title) + "\n")
	meta := fmt.Sprintf("Words read this week: %d", a.wordsReadThisWeek())
	if s.saved.Chapter > 0 && s.phase != storyLoading && s.phase != storyFailed {
		meta = fmt.Sprintf("Chapter %d of %d · ", s.saved.Chapter, teacher.StoryChapters) + meta
	}
	b.WriteString(r.label.Render(meta) + "\n\n")
	if s.err != nil && s.phase != storyFailed {
		b.WriteString(r.danger.Render("✗ "+capitalize(s.err.Error())) + "\n\n")
	}

	switch s.phase {
	case storyLoading:
		next := 1
		if s.saved.Title != "" && !s.finished() {
			next = s.saved.Chapter + 1
		}
		b.WriteString(r.secondary.Render(fmt.Sprintf("Writing chapter %d for you… this takes about 20 seconds.", next)))
	case storyFailed:
		if errors.Is(s.err, errDamagedChapter) {
			b.WriteString(r.danger.Render("✗ Could not read the saved chapter.") + "\n\n")
			b.WriteString(r.label.Render("Press enter for the next chapter."))
			break
		}
		b.WriteString(r.danger.Render("✗ Could not get the chapter: "+s.err.Error()) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
	case storyReading:
		b.WriteString(a.chapterText(w, max(h-strings.Count(b.String(), "\n")-2, 3)))
	case storyGlossary:
		b.WriteString(r.label.Render("Glossary · mark the words to study") + "\n")
		rows := max(h-strings.Count(b.String(), "\n")-1, 3)
		first := max(min(s.cursor-rows/2, len(s.marked)-rows), 0)
		for i := first; i < min(first+rows, len(s.marked)); i++ {
			b.WriteString(a.glossLine(s.chapter.Glossary[i], i == s.cursor, s.marked[i]) + "\n")
		}
	case storyQuestion, storyAnswered:
		b.WriteString(a.chapterQuestion(w))
	}
	return strings.TrimRight(b.String(), "\n")
}

// chapterText is the part of the chapter that fits in rows lines, from
// the scroll position, with a marker when more text follows.
func (a App) chapterText(w, rows int) string {
	r := a.st.main
	s := a.story
	lines := strings.Split(ansi.Wordwrap(strings.TrimSpace(s.saved.Text), max(w, 20), ""), "\n")
	first := min(s.scroll, max(len(lines)-rows, 0))
	last := min(first+rows, len(lines))
	out := r.target.Render(strings.Join(lines[first:last], "\n"))
	if last < len(lines) {
		return out + "\n" + r.label.Render("↓ more")
	}
	return out + "\n" + r.label.Render("Press enter for the glossary.")
}

// glossLine is one row of a glossary: the pointer, the mark, the word,
// its meaning and whether it is new.
func (a App) glossLine(g teacher.Gloss, current, marked bool) string {
	r := a.st.main
	pointer, box := "  ", "[ ] "
	if current {
		pointer = r.accent.Render("▸ ")
	}
	if marked {
		box = "[x] "
	}
	line := pointer + r.value.Render(box+g.Word) + r.secondary.Render(" — "+g.Meaning)
	if g.New {
		line += r.accent.Render("  new")
	}
	return line
}

// chapterQuestion is the question with its options and, once answered,
// whether the learner was right and what comes next.
func (a App) chapterQuestion(w int) string {
	r := a.st.main
	s := a.story
	q := s.chapter.Question
	var b strings.Builder
	b.WriteString(r.target.Render(ansi.Wordwrap(q.Question, max(w, 20), "")) + "\n\n")
	for i, o := range q.Options {
		line := fmt.Sprintf("%d  %s", i+1, o)
		switch {
		case s.phase != storyAnswered:
			b.WriteString(r.value.Render(line) + "\n")
		case i == q.Answer:
			b.WriteString(r.success.Render(line) + "\n")
		case i == s.choice:
			b.WriteString(r.danger.Render(line) + "\n")
		default:
			b.WriteString(r.label.Render(line) + "\n")
		}
	}
	if s.phase != storyAnswered {
		return b.String()
	}
	b.WriteString("\n")
	if s.choice == q.Answer {
		b.WriteString(r.success.Render("✓ Right: "+q.Right()) + "\n")
	} else {
		b.WriteString(r.danger.Render(fmt.Sprintf("✗ Not quite. The answer is %d: %s", q.Answer+1, q.Right())) + "\n")
	}
	if q.Why != "" {
		b.WriteString(r.secondary.Render(ansi.Wordwrap(q.Why, max(w, 20), "")) + "\n")
	}
	b.WriteString("\n")
	if s.finished() {
		b.WriteString(r.success.Render("The end of the story. ") + r.label.Render("Press n for a new story."))
	} else {
		b.WriteString(r.label.Render("Press n for the next chapter."))
	}
	return b.String()
}

// capitalize upper-cases the first letter of an error for a sentence.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (a App) storyHints() []string {
	s := a.story
	switch s.phase {
	case storyLoading:
		return []string{"s short texts", "tab screens", "esc home"}
	case storyFailed:
		return []string{"enter try again", "s short texts", "esc home"}
	case storyReading:
		return []string{"↑↓ scroll", "enter glossary", "s short texts", "esc home"}
	case storyGlossary:
		return []string{"↑↓ move", "space mark", "a done", "s short texts", "esc home"}
	case storyQuestion:
		return []string{"1–3 answer", "s short texts", "esc home"}
	case storyAnswered:
		if s.finished() {
			return []string{"n new story", "s short texts", "esc home"}
		}
		return []string{"n next chapter", "s short texts", "esc home"}
	}
	return []string{"s short texts", "esc home"}
}
