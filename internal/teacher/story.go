package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// StoryChapters is how many chapters a story has; the last one ends it.
const StoryChapters = 5

// StoryOptions is the number of options of a chapter's comprehension
// question, answered with 1 to StoryOptions.
const StoryOptions = 3

// StoryNewWords is how many new words one chapter may add at level. A
// chapter of about 300 words is easy to read when 95–98% of its words
// are known; a beginner gets only MaxNewWords, like a Learn text.
func StoryNewWords(level domain.Level) int {
	if level == domain.A1 {
		return MaxNewWords
	}
	return 6
}

// StorySoFar is what the teacher needs to write the next chapter. The
// zero value starts a new story.
type StorySoFar struct {
	Title   string // "" for a new story
	Chapter int    // how many chapters were already read
	Summary string // the story so far, in a few sentences
}

// StoryChapter is one chapter of a story at the learner's level.
type StoryChapter struct {
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Summary  string   `json:"summary"` // the story so far, this chapter included
	Glossary []Gloss  `json:"glossary"`
	Question QuizItem `json:"question"` // one comprehension question with StoryOptions options
}

// StoryChapter asks the reading model for the next chapter of so: the
// first chapter of a new story when so is zero, else a chapter that
// continues the same story from its title and summary.
func (t *Teacher) StoryChapter(ctx context.Context, level domain.Level, known []string, so StorySoFar) (StoryChapter, error) {
	next := so.Chapter + 1
	var start string
	if so.Title == "" {
		start = fmt.Sprintf("Start a new story with an interesting plot and a short title. Write chapter 1 of %d.", StoryChapters)
	} else {
		start = fmt.Sprintf("Continue the story %q. Write chapter %d of %d. The story so far: %s",
			so.Title, next, StoryChapters, so.Summary)
	}
	if next >= StoryChapters {
		start += " This is the last chapter: end the story."
	}
	r := Request{
		System: learner(level) + jsonOnly +
			`{"title": string, "text": string, "summary": string, ` +
			`"glossary": [{"word": string, "meaning": string, "example": string, "new": bool}], ` +
			`"question": {"question": string, "options": [string], "answer": number, "why": string}}`,
		Prompt: fmt.Sprintf("%s "+
			"Write it for level %s in 250 to 400 words, in short paragraphs. "+
			"Reuse the learner's known words where they fit: %s. "+
			"At least 95%% of the words must be words the learner knows; use at most %d new words or chunks, "+
			"mark them with \"new\": true and make their meaning clear from the context. "+
			"summary is the whole story so far, this chapter included, in at most 60 words. "+
			"The glossary lists the new words first, then the other content words of the chapter, "+
			"with their Spanish meaning and the sentence that uses them. "+
			"question is one comprehension question in English about this chapter with exactly %d options, "+
			"all different and only one correct; answer is the index of the right option, counting from 0; "+
			"why is one short sentence in Spanish that points to the part of the chapter with the answer.",
			start, level, list(known), StoryNewWords(level), StoryOptions),
	}
	var out StoryChapter
	if err := ask(ctx, t.models.Reading, r, &out); err != nil {
		return StoryChapter{}, err
	}
	out.Text = strings.TrimSpace(out.Text)
	if out.Text == "" {
		return StoryChapter{}, errors.New("teacher: the story chapter came back empty")
	}
	q, ok := cleanChoice(out.Question, StoryOptions)
	if !ok {
		return StoryChapter{}, fmt.Errorf("teacher: the story chapter came back without a valid question with %d options", StoryOptions)
	}
	out.Question = q
	// A continued story keeps its title, whatever the model wrote.
	if so.Title != "" {
		out.Title = so.Title
	}
	out.Title = strings.TrimSpace(out.Title)
	if out.Title == "" {
		out.Title = "A story"
	}
	if out.Summary = strings.TrimSpace(out.Summary); out.Summary == "" {
		out.Summary = so.Summary
	}
	out.Glossary = cleanGlossary(out.Glossary, StoryNewWords(level))
	return out, nil
}
