package teacher

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// clozeJSON writes one cloze item as the model would, without a chunk.
func clozeJSON(sentence, answer, point string, yours bool) string {
	return fmt.Sprintf(`{"sentence":%q,"answer":%q,"hint":"","point":%q,"rule":"Una regla.","yours":%t}`,
		sentence, answer, point, yours)
}

func clozeReply(items ...string) string {
	return `{"items":[` + strings.Join(items, ",") + `]}`
}

func TestClozePointsPutTheMistakesFirstThenTheCoreGrammar(t *testing.T) {
	got := ClozePoints(domain.A2, []string{"article", "spelling", "preposition"})

	// Spelling is no grammar point; the core grammar of A2 follows.
	want := []string{"articles (a/an/the)", "prepositions (in/on/at)",
		"past simple", "going to", "comparatives", "countable and uncountable nouns", "prepositions of time and place"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("points = %v\nwant     %v", got, want)
	}
	// At most MaxClozeMistakes of them lead.
	capped := ClozePoints(domain.A2, []string{"article", "be", "preposition"})
	if want := []string{"articles (a/an/the)", "to be (am/is/are)", "past simple"}; !reflect.DeepEqual(capped[:3], want) || hasFold(capped, "prepositions (in/on/at)") {
		t.Errorf("points = %v, want %v then the core grammar", capped, want)
	}
	if got := ClozePoints(domain.A1, nil); !reflect.DeepEqual(got, CoreGrammar(domain.A1)) {
		t.Errorf("without mistakes the points are the core grammar, got %v", got)
	}
}

func TestOwnSentencesSplitsDedupesAndCaps(t *testing.T) {
	texts := []string{"I am from Argentina.", "i am from argentina. ", "", "Tom went home. He was tired!"}

	got := OwnSentences(texts)

	want := []string{"I am from Argentina.", "Tom went home.", "He was tired!"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sentences = %q, want %q", got, want)
	}
	many := make([]string, 30)
	for i := range many {
		many[i] = fmt.Sprintf("Sentence number %d is here.", i)
	}
	if n := len(OwnSentences(many)); n != MaxOwnSentences {
		t.Errorf("got %d sentences, want at most %d", n, MaxOwnSentences)
	}
}

func TestClozeRoundParsesInterleavesAndUsesTheQuizModel(t *testing.T) {
	quiz := &fakeModel{reply: "```json\n" + clozeReply(
		clozeJSON("Tom went home.", "went", "Past simple", true),
		clozeJSON("She saw a cat.", "saw", "past simple", true), // not the learner's sentence
		`{"sentence":"Could you show me a cheaper jacket?","answer":"cheaper","hint":"cheap","point":"Comparatives","rule":"Adjetivo corto + -er.",`+
			`"chunk":{"phrase":" Could you…? ","meaning":"¿Podrías…?","use":"para pedir algo con cortesía","example":"Could you help me?"}}`,
		clozeJSON("They are going to travel.", "going to", "going to", false),
		clozeJSON("No gap here.", "missing", "comparatives", false),    // the answer is not in the sentence
		clozeJSON("We ate pizza.", "ate", "present continuous", false), // not a point of the round
		clozeJSON("Tom wenter home.", "went", "past simple", false),    // the answer is only part of a word
	) + "\n```"}
	other := &fakeModel{err: errors.New("wrong model")}
	tc := New(Models{Reading: other, Writing: other, Conversation: other, Quiz: quiz})

	got, err := tc.ClozeRound(context.Background(), domain.A2, nil, []string{"Tom went home."})

	if err != nil {
		t.Fatal(err)
	}
	if len(quiz.got) != 1 || len(other.got) != 0 {
		t.Fatalf("quiz model calls %d, other calls %d; want 1 and 0", len(quiz.got), len(other.got))
	}
	var answers []string
	for _, it := range got.Items {
		answers = append(answers, it.Answer)
	}
	if want := []string{"went", "cheaper", "going to", "saw"}; !reflect.DeepEqual(answers, want) {
		t.Fatalf("answers = %v, want %v (interleaved, bad items dropped)", answers, want)
	}
	first := got.Items[0]
	if first.Point != "past simple" || !first.Yours || first.Rule != "Una regla." {
		t.Errorf("first item = %+v; want the round's label, yours, and the rule", first)
	}
	if got.Items[3].Yours {
		t.Error("an item is the learner's only when its sentence is one of theirs")
	}
	chunk := got.Items[1].Chunk
	if chunk.Phrase != "Could you…?" || chunk.Use != "para pedir algo con cortesía" || got.Items[1].Hint != "cheap" ||
		got.Items[1].Point != "comparatives" {
		t.Errorf("chunk item = %+v", got.Items[1])
	}
	if !reflect.DeepEqual(got.Points, CoreGrammar(domain.A2)) {
		t.Errorf("points = %v", got.Points)
	}
	r := quiz.got[0]
	for _, s := range []string{"level A2", "past simple, going to", "Tom went home.", "at least 3", `"chunk"`} {
		if !strings.Contains(r.System+r.Prompt, s) {
			t.Errorf("the request lacks %q:\n%s\n%s", s, r.System, r.Prompt)
		}
	}
}

func TestClozeRoundWeightsTheMistakes(t *testing.T) {
	m := &fakeModel{reply: clozeReply(
		clozeJSON("Tom went home.", "went", "past simple", false),
		clozeJSON("I have a cat.", "a", "articles (a/an/the)", false),
		clozeJSON("They are going to travel.", "going to", "going to", false),
		clozeJSON("She has an apple.", "an", "articles (a/an/the)", false),
	)}
	tc := New(models(m))

	got, err := tc.ClozeRound(context.Background(), domain.A2, []string{"article"}, nil)

	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].Answer != "a" {
		t.Errorf("the learner's mistake comes first, got %+v", got.Items[0])
	}
	if !strings.Contains(m.got[0].Prompt, "articles (a/an/the)") {
		t.Errorf("the prompt lacks the mistake:\n%s", m.got[0].Prompt)
	}
	if strings.Contains(m.got[0].Prompt, "Use these sentences") {
		t.Error("with no sentences of the learner, the prompt asks for new ones only")
	}
}

func TestClozeRoundFailsWithTooFewPoints(t *testing.T) {
	m := &fakeModel{reply: clozeReply(
		clozeJSON("Tom went home.", "went", "past simple", false),
		clozeJSON("She is taller than me.", "taller", "comparatives", false),
		clozeJSON("She saw a dog.", "saw", "past simple", false),
	)}

	_, err := New(models(m)).ClozeRound(context.Background(), domain.A2, nil, nil)

	if err == nil || !strings.Contains(err.Error(), "only 2 grammar points") {
		t.Errorf("err = %v, want too few grammar points", err)
	}
}

func TestClozeRoundReportsTheModelError(t *testing.T) {
	m := &fakeModel{err: errors.New("pi exploded")}

	_, err := New(models(m)).ClozeRound(context.Background(), domain.A1, nil, nil)

	if err == nil || !strings.Contains(err.Error(), "pi exploded") {
		t.Errorf("err = %v", err)
	}
}
