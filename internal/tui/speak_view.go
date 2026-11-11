package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// speakTitle heads every page of the Speak screen.
const speakTitle = "Speak · Shadowing"

func (a App) speakView(w int) string {
	width := max(w, 20)
	switch a.speaking.phase {
	case speakReady, speakRecording, speakListening, speakAnswered:
		return a.speakItemView(width)
	case speakResult:
		return a.speakResultView(width)
	}
	return a.speakIntroView(width)
}

// speakIntroView says what shadowing is, or what is missing: audio,
// whisper, or enough short example sentences.
func (a App) speakIntroView(width int) string {
	r := a.st.main
	wrap := func(s string) string { return ansi.Wordwrap(s, width, "") }
	var b strings.Builder
	b.WriteString(r.title.Render(speakTitle) + "\n\n")
	b.WriteString(r.target.Render(wrap("Hear a sentence, then say it. Whisper, on your computer, "+
		"writes what it heard, and each word it understood is marked.")) + "\n\n")
	n := len(a.speakOptions())
	switch {
	case a.speaker == nil:
		b.WriteString(r.warning.Render(audioOffNotice) + "\n")
		b.WriteString(r.label.Render(wrap("Speak plays each sentence first: run `teacher setup voice`, then `teacher doctor` to check that piper and a player are found.")))
	case !a.whisperOn():
		b.WriteString(r.warning.Render("Whisper is off") + "\n")
		if h := a.speaking.offHint; h != "" {
			b.WriteString(r.danger.Render(wrap(h)) + "\n")
		}
		b.WriteString(r.label.Render(wrap(whisperHelp)))
	case n < minDictationItems:
		b.WriteString(r.secondary.Render("Not enough sentences yet.") + "\n")
		b.WriteString(r.label.Render(wrap(fmt.Sprintf(
			"Speak uses the example sentences of your cards, up to %d words long, and needs at least %d. "+
				"Add words with an example sentence in Learn or with `teacher import`.",
			maxDictationWords, minDictationItems))))
	default:
		b.WriteString(r.value.Render(fmt.Sprintf("%d sentences from your cards", n)) + "\n")
		b.WriteString(r.label.Render(wrap("Each one plays when it appears: p plays it again, s slower. " +
			"enter starts recording and enter stops it. Press enter to start.")))
	}
	return b.String()
}

// speakItemView is one sentence: the sentence, the recording state, then
// the marks of the last try.
func (a App) speakItemView(width int) string {
	r := a.st.main
	s := a.speaking
	it := s.items[s.pos]
	var b strings.Builder
	b.WriteString(r.title.Render(speakTitle) + "\n")
	if it.again {
		main := s.mainItems()
		b.WriteString(r.warning.Render(fmt.Sprintf("Practice again · %d of %d", s.pos-main+1, len(s.items)-main)) + "\n\n")
	} else {
		b.WriteString(r.label.Render(fmt.Sprintf("Sentence %d of %d · ✓ %d", s.pos+1, s.mainItems(), s.score())) + "\n\n")
	}

	if s.phase == speakAnswered {
		b.WriteString(r.secondary.Render("What whisper understood") + "\n")
		b.WriteString(indent(ansi.Wordwrap(a.markedSentence(), width-2, ""), 2) + "\n\n")
		heard := r.label.Render("Whisper heard: ") + r.value.Render(s.heard)
		if strings.TrimSpace(s.heard) == "" {
			heard = r.label.Render("Whisper heard nothing. Is your microphone plugged in and turned up?")
		}
		b.WriteString(ansi.Wordwrap(heard, width, "") + "\n")
		var missed []string
		for _, m := range s.marks {
			if !m.Understood {
				missed = append(missed, dictationKey(m.Word))
			}
		}
		if len(missed) == 0 {
			b.WriteString(r.success.Render("✓ Whisper understood every word"))
		} else {
			b.WriteString(r.label.Render("Not understood: ") + r.warning.Render(strings.Join(missed, ", ")))
		}
		next := "r tries again. Press enter for the next sentence."
		if s.pos == len(s.items)-1 && (s.queuedAgain || !s.anyMissed()) {
			next = "r tries again. Press enter to see your result."
		}
		return b.String() + "\n\n" + r.label.Render(ansi.Wordwrap(next, width, ""))
	}

	b.WriteString(r.secondary.Render("Listen, then say it") + "\n")
	b.WriteString(indent(r.target.Render(ansi.Wordwrap(it.sentence, width-2, "")), 2) + "\n\n")
	switch s.phase {
	case speakRecording:
		b.WriteString(r.danger.Render("● Recording… enter to stop") + r.label.Render(" · esc cancels"))
	case speakListening:
		b.WriteString(r.accent.Render("Listening…"))
	default:
		if s.note != "" {
			b.WriteString(r.warning.Render(ansi.Wordwrap(s.note, width, "")) + "\n")
		}
		b.WriteString(r.label.Render(ansi.Wordwrap("Press enter and say it. Wait for the sentence to end first.", width, "")))
	}
	return b.String()
}

// anyMissed reports whether a sentence of the round had a word not
// understood, so practice-again turns follow the last one.
func (s speakState) anyMissed() bool {
	for _, it := range s.items {
		if it.missed {
			return true
		}
	}
	return false
}

// markedSentence is every word of the sentence: understood words in
// green, the others in amber and underlined.
func (a App) markedSentence() string {
	r := a.st.main
	missed := r.warning.Underline(true)
	words := make([]string, len(a.speaking.marks))
	for i, m := range a.speaking.marks {
		if m.Understood {
			words[i] = r.success.Render(m.Word)
		} else {
			words[i] = missed.Render(m.Word)
		}
	}
	return strings.Join(words, " ")
}

// speakResultView is the score and the words to practise.
func (a App) speakResultView(width int) string {
	r := a.st.main
	s := a.speaking
	var b strings.Builder
	b.WriteString(r.title.Render(speakTitle) + "\n\n")
	n, m := s.score(), s.mainItems()
	score := fmt.Sprintf("%d of %d sentences fully understood", n, m)
	if n == m {
		b.WriteString(r.success.Render("✓ " + score))
	} else {
		b.WriteString(r.accent.Render(score))
	}
	b.WriteString(r.label.Render("  ·  saved in today's log") + "\n")
	b.WriteString(r.label.Render(ansi.Wordwrap("A sentence counts when whisper understood every word at the first try.", width, "")) + "\n")
	if len(s.practice) == 0 {
		return b.String() + "\n" + r.success.Render("No words to practise: well said!")
	}
	b.WriteString("\n" + r.secondary.Render("Words to practise") + "\n")
	b.WriteString(indent(r.warning.Render(ansi.Wordwrap(strings.Join(s.practice, ", "), width-2, "")), 2) + "\n")
	b.WriteString(r.label.Render(ansi.Wordwrap("Hear them in Review or Pairs, and say them again in your next round.", width, "")))
	return b.String()
}

// speakHints are the keys of the Speak screen. No text is typed here, so
// p and s are the audio keys.
func (a App) speakHints() []string {
	s := a.speaking
	switch s.phase {
	case speakReady:
		return []string{"enter record", "p again", "s slower", "tab screens", "esc home"}
	case speakRecording:
		return []string{"enter stop", "esc cancel"}
	case speakListening:
		return []string{"listening…"}
	case speakAnswered:
		next := "enter next"
		if s.pos == len(s.items)-1 && (s.queuedAgain || !s.anyMissed()) {
			next = "enter result"
		}
		return []string{next, "r try again", "p again", "s slower", "esc home"}
	case speakResult:
		return []string{"n new round", "tab screens", "esc home"}
	}
	if a.canSpeak() {
		return []string{"enter start", "tab screens", "esc home"}
	}
	return []string{"tab screens", "esc home", "? help"}
}
