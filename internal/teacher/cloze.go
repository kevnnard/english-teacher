package teacher

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
)

// ClozeItems is the most items a cloze round asks (spec S4).
const ClozeItems = 8

// clozeAsked is how many items the model is asked for: a few more than
// ClozeItems, because bad items are dropped and interleaving may stop early.
const clozeAsked = 12

// MaxOwnSentences is the most sentences of the learner's own that a cloze
// request sends, so the prompt stays short as the deck grows.
const MaxOwnSentences = 12

// ClozeQuiz is a cloze round: gap-fill items mixed across grammar points.
type ClozeQuiz struct {
	Points []string           // the grammar points the round was asked to practice
	Items  []domain.ClozeItem // interleaved: never the same point twice in a row
}

// notGrammar are the error kinds a cloze item cannot practice: they are
// about spelling or vocabulary, not grammar.
var notGrammar = map[string]bool{"spelling": true, "punctuation": true, "word-choice": true, OtherKind: true}

// MaxClozeMistakes is the most of the learner's error kinds a cloze round
// puts first, like the focus of a writing task.
const MaxClozeMistakes = 2

// mistakePoints are the labels of the first MaxClozeMistakes grammar kinds
// among the learner's recurring error kinds, most frequent first.
func mistakePoints(mistakes []string) []string {
	var labels []string
	for _, k := range mistakes {
		if len(labels) == MaxClozeMistakes {
			break
		}
		if k = NormalizeKind(k); !notGrammar[k] && !hasFold(labels, KindLabel(k)) {
			labels = append(labels, KindLabel(k))
		}
	}
	return labels
}

// ClozePoints is what a cloze round practices: the grammar points of up to
// MaxClozeMistakes of the learner's recurring error kinds in mistakes (most
// frequent first), then the core grammar of level. Kinds that are not
// grammar, such as spelling, are left out.
func ClozePoints(level domain.Level, mistakes []string) []string {
	points := mistakePoints(mistakes)
	for _, p := range CoreGrammar(level) {
		if !hasFold(points, p) {
			points = append(points, p)
		}
	}
	return points
}

// OwnSentences cuts the learner's texts, such as card examples and a story
// chapter, into sentences for a cloze request. Repeated sentences count
// once, ignoring case and spaces, and at most MaxOwnSentences are kept, the
// first ones first.
func OwnSentences(texts []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range texts {
		for _, s := range splitSentences(t) {
			k := sentenceKey(s)
			if k == "" || seen[k] {
				continue
			}
			if len(out) == MaxOwnSentences {
				return out
			}
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

// sentenceKey is how two sentences are compared: lowercase, single spaces.
func sentenceKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// clozeAnswer is one item as the model writes it.
type clozeAnswer struct {
	Sentence string `json:"sentence"`
	Answer   string `json:"answer"`
	Hint     string `json:"hint"`
	Point    string `json:"point"`
	Rule     string `json:"rule"`
	Chunk    struct {
		Phrase  string `json:"phrase"`
		Meaning string `json:"meaning"`
		Use     string `json:"use"`
		Example string `json:"example"`
	} `json:"chunk"`
}

// ClozeRound asks the quiz model for a cloze round at level. The points are
// ClozePoints(level, mistakes); sentences are the learner's own (see
// OwnSentences), which the model gaps where they fit, writing new ones for
// the rest.
//
// An item is kept only when its answer is whole words of its sentence and
// its point is one of the round's points; the point is then written as the
// round's label. Items are ordered by domain.Interleave, with the learner's
// mistakes weighted first, and fewer than domain.MinClozePoints different
// points is an error.
func (t *Teacher) ClozeRound(ctx context.Context, level domain.Level, mistakes, sentences []string) (ClozeQuiz, error) {
	points := ClozePoints(level, mistakes)
	prompt := fmt.Sprintf("Write %d gap-fill (cloze) items for level %s. Each item is one short English sentence "+
		"with one word or a few words to type in the gap. The grammar points are: %s. "+
		"Mix them: use at least %d different points, and each item practices exactly one of them; "+
		"point is the name of that point, copied from the list. "+
		"answer is the missing word or words exactly as they appear in sentence, and sentence is the whole sentence with the answer in place. "+
		"hint is a short clue shown with the gap, such as the base verb \"go\" for \"went\", or \"\". "+
		"rule is the rule in one short line in Spanish. "+
		"When the sentence has a useful fixed phrase for level %s, such as \"I'm looking for…\" or \"Could you…?\", "+
		"give it in chunk with its meaning in Spanish, its use in a few Spanish words and one example; else leave chunk empty.",
		clozeAsked, level, strings.Join(points, ", "), domain.MinClozePoints, level)
	if len(sentences) > 0 {
		prompt += "\n\nUse these sentences of the learner where they fit a point, copying them exactly; write new ones for the rest:\n- " +
			strings.Join(sentences, "\n- ")
	}
	r := Request{
		System: learner(level) + jsonOnly +
			`{"items": [{"sentence": string, "answer": string, "hint": string, "point": string, "rule": string, ` +
			`"chunk": {"phrase": string, "meaning": string, "use": string, "example": string}}]}`,
		Prompt: prompt,
	}
	var out struct {
		Items []clozeAnswer `json:"items"`
	}
	if err := ask(ctx, t.models.Quiz, r, &out); err != nil {
		return ClozeQuiz{}, err
	}

	own := map[string]bool{}
	for _, s := range sentences {
		own[sentenceKey(s)] = true
	}
	var items []domain.ClozeItem
	for _, it := range out.Items {
		item, ok := cleanCloze(it, points)
		if !ok {
			continue
		}
		item.Yours = own[sentenceKey(item.Sentence)]
		items = append(items, item)
	}
	round, err := domain.Interleave(items, mistakePoints(mistakes), ClozeItems)
	if err != nil {
		return ClozeQuiz{}, fmt.Errorf("teacher: %w", err)
	}
	return ClozeQuiz{Points: points, Items: round}, nil
}

// cleanCloze trims an item and reports whether it is valid: its answer is
// whole words of its sentence and its point is one of points, whose label
// it then takes.
func cleanCloze(it clozeAnswer, points []string) (domain.ClozeItem, bool) {
	item := domain.ClozeItem{
		Sentence: strings.Join(strings.Fields(it.Sentence), " "),
		Answer:   strings.TrimSpace(it.Answer),
		Hint:     strings.TrimSpace(it.Hint),
		Rule:     strings.TrimSpace(it.Rule),
		Chunk: domain.Chunk{
			Phrase: strings.TrimSpace(it.Chunk.Phrase), Meaning: strings.TrimSpace(it.Chunk.Meaning),
			Use: strings.TrimSpace(it.Chunk.Use), Example: strings.TrimSpace(it.Chunk.Example),
		},
	}
	if !item.Chunk.Valid() {
		item.Chunk = domain.Chunk{}
	}
	for _, p := range points {
		if pointName(p) == pointName(it.Point) {
			item.Point = p
		}
	}
	if _, ok := check.Gap(item.Sentence, item.Answer); !ok || item.Answer == "" || item.Point == "" {
		return domain.ClozeItem{}, false
	}
	return item, true
}

// pointName is a point without case, spaces around or a part in brackets,
// so "Articles" names the point "articles (a/an/the)".
func pointName(point string) string {
	if i := strings.Index(point, "("); i >= 0 {
		point = point[:i]
	}
	return strings.ToLower(strings.TrimSpace(point))
}
