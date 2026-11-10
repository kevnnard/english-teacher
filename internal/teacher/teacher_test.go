package teacher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// fakeModel answers every request with reply and remembers the requests.
type fakeModel struct {
	reply string
	err   error
	got   []Request
}

func (f *fakeModel) Complete(_ context.Context, r Request) (string, error) {
	f.got = append(f.got, r)
	return f.reply, f.err
}

func models(m Model) Models {
	return Models{Reading: m, Writing: m, Conversation: m, Quiz: m}
}

func TestReadingParsesTheTextAndGlossary(t *testing.T) {
	m := &fakeModel{reply: "```json\n" + `{"title":"A new friend","text":"Hello! My name is Ana. I am a teacher.",
		"glossary":[{"word":"teacher","meaning":"profesor/a","example":"I am a teacher.","new":true},
		            {"word":"hello","meaning":"hola","example":"Hello!","new":false}]}` + "\n```"}
	tc := New(models(m))

	r, err := tc.Reading(context.Background(), domain.A1, []string{"hello", "name"})

	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "A new friend" || !strings.Contains(r.Text, "My name is Ana") || len(r.Glossary) != 2 {
		t.Fatalf("reading = %+v", r)
	}
	if g := r.Glossary[0]; g.Word != "teacher" || g.Meaning != "profesor/a" || !g.New {
		t.Errorf("glossary[0] = %+v", g)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, want := range []string{"A1", "hello, name", "at most 3 new"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
}

func TestReadingRejectsAnEmptyText(t *testing.T) {
	tc := New(models(&fakeModel{reply: `{"title":"x","text":"  ","glossary":[]}`}))
	if _, err := tc.Reading(context.Background(), domain.A1, nil); err == nil {
		t.Error("want an error for an empty text")
	}
}

func TestBadJSONIsAClearError(t *testing.T) {
	tc := New(models(&fakeModel{reply: "Sure! Here is a text about cats."}))
	_, err := tc.Reading(context.Background(), domain.A1, nil)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("err = %v, want a JSON error", err)
	}
}

func TestModelErrorsAreWrapped(t *testing.T) {
	tc := New(models(&fakeModel{err: errors.New("pi: not found")}))
	_, err := tc.Correct(context.Background(), domain.A1, "Introduce yourself", "I has 30 years.")
	if err == nil || !strings.Contains(err.Error(), "pi: not found") {
		t.Errorf("err = %v", err)
	}
}

func TestMissingModelIsAnError(t *testing.T) {
	tc := New(Models{})
	if _, err := tc.WritingTask(context.Background(), domain.A1, nil, nil); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
	if _, err := tc.Judge(context.Background(), domain.A1, []Attempt{{Wrong: "a", Right: "b", Attempt: "c"}}); !errors.Is(err, ErrNoModel) {
		t.Errorf("Judge err = %v, want ErrNoModel", err)
	}
}

func TestWritingTaskBringsUsefulPhrasesAndTargetsTheFocus(t *testing.T) {
	m := &fakeModel{reply: `{"task":"Write 4 sentences about your family.","hint":"Usá: I have, my brother is...",
		"phrases":[" I have ","my brother is","", "he is 20 years old","I live with","she works in"]}`}

	task, err := New(models(m)).WritingTask(context.Background(), domain.A1, []string{"brother"}, []string{"be", "article"})

	if err != nil || task.Task != "Write 4 sentences about your family." || !strings.HasPrefix(task.Hint, "Usá") {
		t.Fatalf("task = %+v, %v", task, err)
	}
	want := []string{"I have", "my brother is", "he is 20 years old", "I live with"}
	if strings.Join(task.Phrases, "|") != strings.Join(want, "|") {
		t.Errorf("phrases = %q, want %q (trimmed, no blanks, at most %d)", task.Phrases, want, MaxPhrases)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, want := range []string{"brother", "phrases", "to be (am/is/are)", "articles (a/an/the)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
}

func TestWritingTaskWithoutFocusDoesNotMentionErrors(t *testing.T) {
	m := &fakeModel{reply: `{"task":"Write about your day.","hint":"Empezá con: I get up..."}`}
	if _, err := New(models(m)).WritingTask(context.Background(), domain.A1, nil, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.got[0].Prompt, "recurring errors") {
		t.Errorf("prompt mentions errors without focus:\n%s", m.got[0].Prompt)
	}
}

func TestCorrectParsesTheRubricAndClampsIt(t *testing.T) {
	m := &fakeModel{reply: `{"corrected":"I am 30 years old.","rubric":{"grammar":8,"vocabulary":14,"coherence":-2,"task":10},
		"comment":"¡Bien!","fixes":[{"wrong":"I has 30 years","right":"I am 30 years old","meaning":"tengo 30 años",
		"why":"La edad va con to be.","major":true,"kind":"be"}]}`}

	c, err := New(models(m)).Correct(context.Background(), domain.A1, "Introduce yourself", "I has 30 years.")

	if err != nil {
		t.Fatal(err)
	}
	if c.Corrected != "I am 30 years old." || c.Comment != "¡Bien!" || len(c.Fixes) != 1 {
		t.Fatalf("correction = %+v", c)
	}
	if want := (Rubric{Grammar: 8, Vocabulary: 10, Coherence: 0, Task: 10}); c.Rubric != want {
		t.Errorf("rubric = %+v, want %+v", c.Rubric, want)
	}
	if c.Total() != 28 || c.Grade() != "C" {
		t.Errorf("total %d grade %s, want 28 C", c.Total(), c.Grade())
	}
	if f := c.Fixes[0]; !f.Major || f.Kind != "be" || f.Meaning != "tengo 30 años" {
		t.Errorf("fix = %+v", f)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, want := range []string{"I has 30 years.", "Introduce yourself", "grammar", "major", "verb-tense", "word-choice"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestCorrectNormalizesUnknownKinds(t *testing.T) {
	m := &fakeModel{reply: `{"corrected":"x","fixes":[{"wrong":"a","right":"b","kind":" Article "},
		{"wrong":"c","right":"d","kind":"collocation"},{"wrong":"e","right":"f"}]}`}
	c, err := New(models(m)).Correct(context.Background(), domain.A1, "t", "x")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, f := range c.Fixes {
		kinds = append(kinds, f.Kind)
	}
	if strings.Join(kinds, ",") != "article,other,other" {
		t.Errorf("kinds = %v", kinds)
	}
}

func TestCorrectDropsFixesThatChangeNothing(t *testing.T) {
	m := &fakeModel{reply: `{"corrected":"Hi.","fixes":[{"wrong":"Hi","right":"Hi","why":"ok"},{"wrong":"","right":"x"}]}`}
	c, err := New(models(m)).Correct(context.Background(), domain.A1, "t", "Hi.")
	if err != nil || len(c.Fixes) != 0 {
		t.Errorf("fixes = %+v, %v", c.Fixes, err)
	}
}

func TestGrade(t *testing.T) {
	tests := []struct {
		total int
		want  string
	}{{40, "A"}, {36, "A"}, {35, "B"}, {32, "B"}, {31, "C"}, {28, "C"}, {27, "D"}, {24, "D"}, {23, "F"}, {0, "F"}}
	for _, tt := range tests {
		c := Correction{Rubric: Rubric{Grammar: min(tt.total, 10), Vocabulary: min(max(tt.total-10, 0), 10),
			Coherence: min(max(tt.total-20, 0), 10), Task: max(tt.total-30, 0)}}
		if c.Total() != tt.total || c.Grade() != tt.want {
			t.Errorf("total %d: got %d %s, want %s", tt.total, c.Total(), c.Grade(), tt.want)
		}
	}
}

func fix(wrong string, major bool) Fix {
	return Fix{Wrong: wrong, Right: wrong + "!", Major: major}
}

func wrongs(fs []Fix) string {
	var s []string
	for _, f := range fs {
		s = append(s, f.Wrong)
	}
	return strings.Join(s, ",")
}

func TestMajorsAndMinors(t *testing.T) {
	tests := []struct {
		name          string
		fixes         []Fix
		majors, minor string
	}{
		{"marked", []Fix{fix("a", false), fix("b", true), fix("c", false), fix("d", true)}, "b,d", "a,c"},
		{"at most 3", []Fix{fix("a", true), fix("b", true), fix("c", true), fix("d", true)}, "a,b,c", "d"},
		{"none marked means no majors", []Fix{fix("a", false), fix("b", false)}, "", "a,b"},
		{"no fixes", nil, "", ""},
	}
	for _, tt := range tests {
		c := Correction{Fixes: tt.fixes}
		if got := wrongs(c.Majors()); got != tt.majors {
			t.Errorf("%s: majors = %s, want %s", tt.name, got, tt.majors)
		}
		if got := wrongs(c.Minors()); got != tt.minor {
			t.Errorf("%s: minors = %s, want %s", tt.name, got, tt.minor)
		}
	}
}

func TestKindsHaveLabels(t *testing.T) {
	want := "be,verb-tense,verb-form,agreement,word-order,article,preposition,plural,pronoun,word-choice,spelling,punctuation,other"
	var names []string
	for _, k := range Kinds {
		names = append(names, k.Name)
		if k.Label == "" {
			t.Errorf("kind %s has no label", k.Name)
		}
	}
	if strings.Join(names, ",") != want {
		t.Errorf("kinds = %v", names)
	}
	if KindLabel("be") != "to be (am/is/are)" || KindLabel("nope") != KindLabel("other") {
		t.Errorf("labels: be=%q nope=%q", KindLabel("be"), KindLabel("nope"))
	}
}

func TestJudgeSendsEveryAttemptInOneCall(t *testing.T) {
	m := &fakeModel{reply: `{"fixed":[true,false]}`}
	attempts := []Attempt{
		{Sentence: "I has 30 years.", Wrong: "I has 30 years", Right: "I am 30 years old", Attempt: "I'm 30 years old"},
		{Sentence: "I go to home.", Wrong: "go to home", Right: "go home", Attempt: "go at home"},
	}

	got, err := New(models(m)).Judge(context.Background(), domain.A1, attempts)

	if err != nil || len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("Judge = %v, %v", got, err)
	}
	if len(m.got) != 1 {
		t.Fatalf("%d calls, want 1", len(m.got))
	}
	for _, want := range []string{"I'm 30 years old", "go at home", "I go to home.", "I am 30 years old", "valid"} {
		if !strings.Contains(m.got[0].System+m.got[0].Prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestJudgeRejectsAnAnswerOfTheWrongLength(t *testing.T) {
	m := &fakeModel{reply: `{"fixed":[true]}`}
	_, err := New(models(m)).Judge(context.Background(), domain.A1, []Attempt{{Attempt: "a"}, {Attempt: "b"}})
	if err == nil {
		t.Error("want an error when the answer does not match the attempts")
	}
}

func TestJudgeWithNoAttemptsAsksNothing(t *testing.T) {
	m := &fakeModel{}
	got, err := New(models(m)).Judge(context.Background(), domain.A1, nil)
	if err != nil || len(got) != 0 || len(m.got) != 0 {
		t.Errorf("Judge = %v, %v with %d calls", got, err, len(m.got))
	}
}

func TestFixCard(t *testing.T) {
	f := Fix{Wrong: "I has 30 years", Right: "I am 30 years old", Meaning: "tengo 30 años", Why: "La edad va con to be."}

	c := f.Card("I has 30 years and I am from Argentina.")

	if c.Word != "I am 30 years old" || c.Meaning != "tengo 30 años" || c.LastWrong != "I has 30 years" {
		t.Errorf("card = %+v", c)
	}
	if c.Example != "I has 30 years and I am from Argentina." || c.EaseFactor != domain.DefaultEaseFactor {
		t.Errorf("card = %+v", c)
	}
}

func TestGlossCard(t *testing.T) {
	c := Gloss{Word: "teacher", Meaning: "profesor/a", Example: "I am a teacher."}.Card()
	if c.Word != "teacher" || c.Meaning != "profesor/a" || c.Example != "I am a teacher." || c.LastReviewed != nil {
		t.Errorf("card = %+v", c)
	}
}

func TestReadingKeepsAtMostMaxNewWordsNew(t *testing.T) {
	m := &fakeModel{reply: `{"title":"t","text":"x","glossary":[
		{"word":"a","meaning":"1","new":true},{"word":"b","meaning":"2","new":true},
		{"word":"c","meaning":"3","new":true},{"word":"d","meaning":"4","new":true},{"word":"e","meaning":"5"}]}`}

	r, err := New(models(m)).Reading(context.Background(), domain.A1, nil)

	if err != nil || len(r.Glossary) != 5 {
		t.Fatalf("reading = %+v, %v", r, err)
	}
	news := 0
	for _, g := range r.Glossary {
		if g.New {
			news++
		}
	}
	if news != MaxNewWords || r.Glossary[3].New {
		t.Errorf("%d new (d new: %v), want the first %d only", news, r.Glossary[3].New, MaxNewWords)
	}
}

func TestWritingTaskFillsPhrasesFromRecentWords(t *testing.T) {
	tests := []struct {
		name, reply string
		recent      []string
		want        string
	}{
		{"none from the model", `{"task":"t","hint":"h","phrases":[]}`, []string{"brother", "I'm from", "dog"}, "brother|I'm from"},
		{"one from the model", `{"task":"t","hint":"h","phrases":["My name is"]}`, []string{"my name is", "dog"}, "My name is|dog"},
		{"enough already", `{"task":"t","hint":"h","phrases":["a","b","c"]}`, []string{"dog"}, "a|b|c"},
		{"empty deck", `{"task":"t","hint":"h"}`, nil, ""},
		{"duplicate from the model", `{"task":"t","hint":"h","phrases":["Hello","hello"]}`, []string{"HELLO", "dog"}, "Hello|dog"},
	}
	for _, tt := range tests {
		task, err := New(models(&fakeModel{reply: tt.reply})).WritingTask(context.Background(), domain.A1, tt.recent, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(task.Phrases, "|"); got != tt.want {
			t.Errorf("%s: phrases = %q, want %q", tt.name, got, tt.want)
		}
	}
}
