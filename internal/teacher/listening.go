package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Extensive listening (spec S2): a text of two to five minutes of speech
// that the learner hears before reading it, with a few comprehension
// questions. It comes back one sentence per line, so the app can play,
// pause and replay it sentence by sentence.

// ListeningKind is what a listening text is: a dialogue or a story.
type ListeningKind string

// The kinds of listening text.
const (
	ListeningDialogue ListeningKind = "dialogue" // two people talk
	ListeningStory    ListeningKind = "story"    // one narrator tells a story
)

// ListeningNarrator is the speaker of every line of a story.
const ListeningNarrator = "Narrator"

// ListeningOptions is the number of options of each question, answered
// with 1 to ListeningOptions.
const ListeningOptions = 3

// MinListeningQuestions and MaxListeningQuestions bound how many valid
// questions a listening text has; fewer than the minimum is an error.
const (
	MinListeningQuestions = 3
	MaxListeningQuestions = 5
)

// ListeningLine is one sentence and who says it.
type ListeningLine struct {
	Speaker string `json:"speaker"` // a name in a dialogue; ListeningNarrator in a story
	Text    string `json:"text"`    // exactly one sentence
}

// Listening is a text to hear and the questions about it.
type Listening struct {
	Title     string
	Kind      ListeningKind
	Lines     []ListeningLine // one sentence each, in the order they are said
	Questions []QuizItem      // MinListeningQuestions to MaxListeningQuestions valid items
}

// ListeningText asks the reading model for a dialogue or a story of about
// 250 to 600 words at level, with the same limits on new words as a story
// chapter, and 3 to 5 comprehension questions with ListeningOptions
// options. Lines with several sentences are split, blank lines are
// dropped and invalid questions are dropped like in GrammarQuiz.
func (t *Teacher) ListeningText(ctx context.Context, level domain.Level, known []string, kind ListeningKind) (Listening, error) {
	what := "a natural dialogue between 2 people with first names; speaker is the name of who says each line"
	if kind == ListeningStory {
		what = "a short story told by one narrator; speaker is always \"" + ListeningNarrator + "\""
	}
	r := Request{
		System: learner(level) + jsonOnly +
			`{"title": string, "lines": [{"speaker": string, "text": string}], ` +
			`"questions": [{"question": string, "options": [string], "answer": number, "why": string}]}`,
		Prompt: fmt.Sprintf("This is a listening exercise: the learner hears it read aloud before reading it. "+
			"Write %s, with a short title, for level %s in 250 to 600 words (2 to 5 minutes of speech). "+
			"Each line is one short sentence. "+
			"Reuse the learner's known words where they fit: %s. "+
			"At least 95%% of the words must be words the learner knows; use at most %d new words or chunks, "+
			"and make their meaning clear from the context. "+
			"Then write %d to %d comprehension questions in English about what is said, each with exactly %d options, "+
			"all different and only one correct; answer is the index of the right option, counting from 0; "+
			"why is one short sentence in Spanish that points to the part of the text with the answer.",
			what, level, list(known), StoryNewWords(level),
			MinListeningQuestions, MaxListeningQuestions, ListeningOptions),
	}
	var out struct {
		Title     string          `json:"title"`
		Lines     []ListeningLine `json:"lines"`
		Questions []QuizItem      `json:"questions"`
	}
	if err := ask(ctx, t.models.Reading, r, &out); err != nil {
		return Listening{}, err
	}
	lines := cleanLines(out.Lines, kind)
	if len(lines) == 0 {
		return Listening{}, errors.New("teacher: the listening text came back empty")
	}
	var questions []QuizItem
	for _, q := range out.Questions {
		if q, ok := cleanChoice(q, ListeningOptions); ok && len(questions) < MaxListeningQuestions {
			questions = append(questions, q)
		}
	}
	if len(questions) < MinListeningQuestions {
		return Listening{}, fmt.Errorf("teacher: the listening text came back with only %d valid questions", len(questions))
	}
	title := strings.TrimSpace(out.Title)
	if title == "" {
		title = "Listening"
	}
	return Listening{Title: title, Kind: kind, Lines: lines, Questions: questions}, nil
}

// cleanLines splits each line into its sentences and drops blank ones.
// Every line of a story is the narrator's; a dialogue line without a
// speaker keeps the speaker of the line before.
func cleanLines(in []ListeningLine, kind ListeningKind) []ListeningLine {
	var out []ListeningLine
	speaker := ListeningNarrator
	for _, l := range in {
		sentences := splitSentences(l.Text)
		if len(sentences) == 0 {
			continue // a blank line says nothing, so its speaker does not count
		}
		if s := strings.TrimSpace(l.Speaker); s != "" && kind != ListeningStory {
			speaker = s
		}
		for _, sentence := range sentences {
			out = append(out, ListeningLine{Speaker: speaker, Text: sentence})
		}
	}
	return out
}

// abbreviations end in a period but do not end a sentence.
var abbreviations = map[string]bool{"mr.": true, "mrs.": true, "ms.": true, "dr.": true, "st.": true}

// splitSentences cuts text after each word that ends in . ! or ?, maybe
// followed by a closing quote or bracket, unless the word is a title such
// as "Mr.". Spaces inside a sentence become single spaces.
func splitSentences(text string) []string {
	var sentences, words []string
	for _, w := range strings.Fields(text) {
		words = append(words, w)
		end := strings.TrimRight(w, `"')]”’`)
		if strings.ContainsAny(end[max(len(end)-1, 0):], ".!?") && !abbreviations[strings.ToLower(end)] {
			sentences = append(sentences, strings.Join(words, " "))
			words = nil
		}
	}
	if len(words) > 0 {
		sentences = append(sentences, strings.Join(words, " "))
	}
	return sentences
}
