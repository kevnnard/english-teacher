package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// MaxNewWords is how many unknown words a reading may add. Reading only
// teaches when about 98% of the words are already known (Nation), so a
// short text gets very few new ones.
const MaxNewWords = 3

// Reading is a short text at the learner's level.
type Reading struct {
	Title    string  `json:"title"`
	Text     string  `json:"text"`
	Glossary []Gloss `json:"glossary"`
}

// Gloss is one word or chunk of a reading with its meaning in Spanish.
type Gloss struct {
	Word    string `json:"word"`    // as it appears in the text, base form
	Meaning string `json:"meaning"` // in Spanish
	Example string `json:"example"` // the sentence of the text that uses it
	New     bool   `json:"new"`     // one of the few words the learner does not know yet
}

// Card turns the gloss into a new vocabulary card.
func (g Gloss) Card() domain.Card {
	c := domain.NewCard(strings.TrimSpace(g.Word))
	c.Meaning, c.Example = strings.TrimSpace(g.Meaning), strings.TrimSpace(g.Example)
	return c
}

// Reading asks for a short text that reuses known words and adds at most
// MaxNewWords new words or chunks.
func (t *Teacher) Reading(ctx context.Context, level domain.Level, known []string) (Reading, error) {
	r := Request{
		System: learner(level) + jsonOnly +
			`{"title": string, "text": string, "glossary": [{"word": string, "meaning": string, "example": string, "new": bool}]}`,
		Prompt: fmt.Sprintf("Write a short, interesting text (60 to 100 words, simple sentences) for level %s. "+
			"Reuse as many of the learner's known words as fit naturally: %s. "+
			"Use at most %d new words or useful chunks (like \"how about\" or \"I'm from\"), mark them with \"new\": true, "+
			"and make the rest of the text easy enough that the learner knows about 98%% of the words. "+
			"The glossary lists the new words first, then every other content word or chunk of the text "+
			"with its Spanish meaning and the sentence of the text that uses it.",
			level, list(known), MaxNewWords),
	}
	var out Reading
	if err := ask(ctx, t.models.Reading, r, &out); err != nil {
		return Reading{}, err
	}
	if strings.TrimSpace(out.Text) == "" {
		return Reading{}, errors.New("teacher: the reading came back empty")
	}
	out.Glossary = cleanGlossary(out.Glossary, MaxNewWords)
	return out, nil
}

// cleanGlossary drops entries without a word or a meaning. The model may
// ignore the limit of new words, so only the first maxNew new entries
// stay new: the others are still listed, just not pre-marked.
func cleanGlossary(entries []Gloss, maxNew int) []Gloss {
	var glossary []Gloss
	news := 0
	for _, g := range entries {
		if strings.TrimSpace(g.Word) == "" || strings.TrimSpace(g.Meaning) == "" {
			continue
		}
		if g.New {
			news++
			g.New = news <= maxNew
		}
		glossary = append(glossary, g)
	}
	return glossary
}
