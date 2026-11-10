package teacher

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestScenariosAreTheFourA1RolePlaysInOrder(t *testing.T) {
	var titles []string
	for _, s := range Scenarios {
		titles = append(titles, s.Title)
		if s.ID == "" || s.Partner == "" || s.Role == "" || s.Goal == "" || s.Opening == "" {
			t.Errorf("scenario %q lacks a field: %+v", s.Title, s)
		}
		if n := len(s.Frames); n < 3 || n > 5 {
			t.Errorf("scenario %q has %d frames, want 3 to 5", s.Title, n)
		}
	}
	want := []string{"Introduce yourself", "Order a coffee", "Ask for directions", "Small talk at work"}
	if !reflect.DeepEqual(titles, want) {
		t.Errorf("titles = %v, want %v", titles, want)
	}
}

func coffee() Scenario {
	return Scenario{ID: "coffee", Title: "Order a coffee", Partner: "Barista", Role: "a barista",
		Goal: "Order a drink and pay.", Opening: "Hi! What can I get you?",
		Frames: []string{"Can I have a ___, please?", "A ___ one, please.", "Excuse me, how much is it?"}}
}

func TestFramesFadeWithLevel(t *testing.T) {
	s := coffee()
	tests := []struct {
		level domain.Level
		want  []string
	}{
		{domain.A1, []string{"Can I have a ___, please?", "A ___ one, please.", "Excuse me, how much is it?"}},
		// The first two words, or one when the second is already a blank.
		{domain.A2, []string{"Can I…", "A…", "Excuse me…"}},
		{domain.B1, nil},
		{domain.C2, nil},
	}
	for _, tt := range tests {
		if got := s.FramesFor(tt.level); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s frames = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func transcript() []Line {
	return []Line{
		{Tutor: true, Text: "Hi! What can I get you?"},
		{Text: "I want a coffee big."},
	}
}

func TestRolePlaySendsTheScenarioLevelAndTranscript(t *testing.T) {
	m := &fakeModel{reply: `{"reply":"A big coffee, sure! Milk?","recast":"I'd like a big coffee.","done":false}`}
	tc := New(Models{Conversation: m})

	turn, err := tc.RolePlay(context.Background(), domain.A1, coffee(), transcript())

	if err != nil {
		t.Fatal(err)
	}
	want := Turn{Reply: "A big coffee, sure! Milk?", Recast: "I'd like a big coffee."}
	if turn != want {
		t.Errorf("turn = %+v, want %+v", turn, want)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, w := range []string{"A1", "a barista", "Order a drink and pay.", "Barista: Hi! What can I get you?",
		"Learner: I want a coffee big.", "in character", "never", `"recast"`, `"done"`} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt lacks %q:\n%s", w, prompt)
		}
	}
}

func TestRolePlayDropsARecastThatChangesNothing(t *testing.T) {
	m := &fakeModel{reply: `{"reply":"Great!","recast":"i want a coffee big","done":true}`}
	turn, err := New(Models{Conversation: m}).RolePlay(context.Background(), domain.A1, coffee(), transcript())
	if err != nil {
		t.Fatal(err)
	}
	if turn.Recast != "" || !turn.Done {
		t.Errorf("turn = %+v, want no recast and done", turn)
	}
}

func TestRolePlayRejectsAnEmptyReply(t *testing.T) {
	m := &fakeModel{reply: `{"reply":"  ","recast":"","done":false}`}
	if _, err := New(Models{Conversation: m}).RolePlay(context.Background(), domain.A1, coffee(), transcript()); err == nil {
		t.Error("want an error for an empty reply")
	}
}

func TestRolePlayUsesTheConversationModel(t *testing.T) {
	tc := New(Models{Writing: &fakeModel{reply: `{"reply":"Hi"}`}})
	if _, err := tc.RolePlay(context.Background(), domain.A1, coffee(), transcript()); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
	if _, err := tc.RolePlayFeedback(context.Background(), domain.A1, coffee(), transcript()); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
}

func TestRolePlayFeedbackParsesPointsAndFixes(t *testing.T) {
	m := &fakeModel{reply: `{"points":[" Pediste el café con claridad. ","","Ojo con el orden: adjetivo antes.",
		"Usá please.","Cuarto punto de más."],
		"fixes":[{"wrong":"a coffee big","right":"a big coffee","meaning":"un café grande","why":"El adjetivo va antes.",
		"major":true,"kind":"Word-Order"},
		{"wrong":"I want","right":"I want","why":"igual","major":true,"kind":"word-choice"},
		{"wrong":"coffee","right":"coffee,","why":"coma","major":false,"kind":"nonsense"}]}`}
	tc := New(Models{Conversation: m})

	fb, err := tc.RolePlayFeedback(context.Background(), domain.A1, coffee(), transcript())

	if err != nil {
		t.Fatal(err)
	}
	wantPoints := []string{"Pediste el café con claridad.", "Ojo con el orden: adjetivo antes.", "Usá please."}
	if !reflect.DeepEqual(fb.Points, wantPoints) {
		t.Errorf("points = %q, want %q", fb.Points, wantPoints)
	}
	wantFixes := []Fix{
		{Wrong: "a coffee big", Right: "a big coffee", Meaning: "un café grande", Why: "El adjetivo va antes.",
			Major: true, Kind: "word-order"},
		{Wrong: "coffee", Right: "coffee,", Why: "coma", Kind: OtherKind},
	}
	if !reflect.DeepEqual(fb.Fixes, wantFixes) {
		t.Errorf("fixes = %+v, want %+v", fb.Fixes, wantFixes)
	}
	if majors := fb.Majors(); len(majors) != 1 || majors[0].Right != "a big coffee" {
		t.Errorf("majors = %+v", majors)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, w := range []string{"Spanish", "Learner: I want a coffee big.", "Order a drink and pay.", "word-order"} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt lacks %q:\n%s", w, prompt)
		}
	}
}

func TestRolePlayFeedbackNeedsAPoint(t *testing.T) {
	m := &fakeModel{reply: `{"points":[],"fixes":[]}`}
	if _, err := New(Models{Conversation: m}).RolePlayFeedback(context.Background(), domain.A1, coffee(), transcript()); err == nil {
		t.Error("want an error for a summary with no points")
	}
}
