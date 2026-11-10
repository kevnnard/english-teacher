package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// MaxPoints is the most points a role-play feedback summary keeps.
const MaxPoints = 3

// Scenario is one guided role-play: who the tutor plays, what the learner
// has to do and the sentence frames that help a beginner say it.
type Scenario struct {
	ID      string   // a stable identifier, such as "coffee"
	Title   string   // the English title the learner picks from
	Partner string   // the short name of the tutor's character, shown in the transcript
	Role    string   // who the tutor plays, for the model
	Goal    string   // what the learner has to do, in English
	Opening string   // the tutor's first line, so the role-play starts without waiting
	Frames  []string // 3 to 5 sentence frames; "___" is the blank the learner fills
}

// Scenarios are the guided A1 role-plays, in the order the learner sees them.
var Scenarios = []Scenario{
	{
		ID: "intro", Title: "Introduce yourself", Partner: "Sam",
		Role:    "Sam, a friendly new classmate at an English course, meeting the learner for the first time",
		Goal:    "Say your name, where you are from and what you do.",
		Opening: "Hi! I'm Sam. What's your name?",
		Frames:  []string{"My name is ___.", "I'm from ___.", "I'm a ___.", "Nice to meet you, ___."},
	},
	{
		ID: "coffee", Title: "Order a coffee", Partner: "Barista",
		Role:    "a barista at a busy coffee shop",
		Goal:    "Order a drink, choose the size and pay.",
		Opening: "Hi there! What can I get you?",
		Frames:  []string{"Can I have a ___, please?", "A ___ one, please.", "How much is it?", "Here you are."},
	},
	{
		ID: "directions", Title: "Ask for directions", Partner: "Local",
		Role:    "a helpful local on a city street",
		Goal:    "Find out how to get to the train station.",
		Opening: "Hi! Are you OK? You look lost.",
		Frames:  []string{"Excuse me, where is the ___?", "Is it near the ___?", "Can you say that again, please?", "Thank you very much!"},
	},
	{
		ID: "smalltalk", Title: "Small talk at work", Partner: "Alex",
		Role:    "Alex, a friendly coworker in the office kitchen",
		Goal:    "Chat about your weekend and your work for a minute.",
		Opening: "Morning! How was your weekend?",
		Frames:  []string{"It was ___, thanks.", "I ___ on Saturday.", "How about you?", "I work on ___."},
	},
}

// FramesFor is the help the frames give at level, which fades as the
// learner grows: A1 gets every frame with its blanks, A2 only the first
// word or two of each ("Can I…") and B1 and above no frames, only the goal.
func (s Scenario) FramesFor(level domain.Level) []string {
	switch level {
	case domain.A1:
		return s.Frames
	case domain.A2:
		starts := make([]string, len(s.Frames))
		for i, f := range s.Frames {
			starts[i] = frameStart(f)
		}
		return starts
	}
	return nil
}

// frameStart is the first two words of frame, or only the first when the
// second is a blank, without trailing punctuation, followed by "…".
func frameStart(frame string) string {
	words := strings.Fields(frame)
	n := min(2, len(words))
	if n == 2 && strings.Contains(words[1], "___") {
		n = 1
	}
	return strings.TrimRight(strings.Join(words[:n], " "), ",.!?") + "…"
}

// Line is one line of a role-play transcript.
type Line struct {
	Tutor  bool   // the tutor said it; otherwise the learner did
	Text   string // what was said
	Recast string // learner lines only: the line said correctly, when it had errors
}

// Turn is the tutor's answer to the learner's last line.
type Turn struct {
	Reply  string `json:"reply"`  // the tutor's answer, in character
	Recast string `json:"recast"` // the learner's line said correctly; "" when it was right
	Done   bool   `json:"done"`   // the learner has met the scenario's goal
}

// Feedback is the summary at the end of a role-play.
type Feedback struct {
	Points []string `json:"points"` // up to MaxPoints short points in Spanish
	Fixes  []Fix    `json:"fixes"`  // the learner's errors, in the shape of a writing correction
}

// Majors are the at most MaxMajors errors marked major, in order.
func (f Feedback) Majors() []Fix {
	return Correction{Fixes: f.Fixes}.Majors()
}

// scenarioBrief tells the model the role-play it is in.
func scenarioBrief(s Scenario) string {
	return fmt.Sprintf(" You are in a role-play with the learner. Your role: %s. The learner's goal: %s",
		s.Role, s.Goal)
}

// transcriptText writes the transcript for a prompt, one line per turn.
func transcriptText(s Scenario, lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		who := "Learner"
		if l.Tutor {
			who = s.Partner
		}
		fmt.Fprintf(&b, "%s: %s\n", who, l.Text)
	}
	return b.String()
}

// RolePlay asks the tutor for the next turn. The tutor answers in
// character and recasts an error by saying the learner's line correctly
// inside the reply, without stopping the conversation: the learner hears
// the right form and keeps talking.
func (t *Teacher) RolePlay(ctx context.Context, level domain.Level, s Scenario, lines []Line) (Turn, error) {
	r := Request{
		System: learner(level) + scenarioBrief(s) + jsonOnly + `{"reply": string, "recast": string, "done": boolean}`,
		Prompt: "Answer the learner's last line in character: one or two short sentences at the learner's level, " +
			"and keep the conversation going toward the goal. If the learner's last line has errors, recast is " +
			"the learner's line said correctly, and your reply uses that correct form naturally; you never say " +
			"the learner made a mistake and never explain grammar. If the line is correct, recast is \"\". " +
			"done is true only when the learner has met the goal.\n\nTranscript:\n" + transcriptText(s, lines),
	}
	var out Turn
	if err := ask(ctx, t.models.Conversation, r, &out); err != nil {
		return Turn{}, err
	}
	out.Reply, out.Recast = strings.TrimSpace(out.Reply), strings.TrimSpace(out.Recast)
	if out.Reply == "" {
		return Turn{}, errors.New("teacher: the tutor's reply came back empty")
	}
	if len(lines) > 0 && sameLine(out.Recast, lines[len(lines)-1].Text) {
		out.Recast = ""
	}
	return out, nil
}

// sameLine reports whether a and b say the same, ignoring case and the
// final punctuation, so a recast that changes nothing is dropped.
func sameLine(a, b string) bool {
	trim := func(s string) string { return strings.TrimRight(strings.TrimSpace(s), ".!?") }
	return strings.EqualFold(trim(a), trim(b))
}

// RolePlayFeedback sums up a finished role-play: a few points in Spanish
// on what went well and what to work on, and the learner's errors as
// fixes, so the major ones can become cards.
func (t *Teacher) RolePlayFeedback(ctx context.Context, level domain.Level, s Scenario, lines []Line) (Feedback, error) {
	r := Request{
		System: learner(level) + scenarioBrief(s) + jsonOnly +
			`{"points": [string], "fixes": [{"wrong": string, "right": string, "meaning": string, "why": string, ` +
			`"major": boolean, "kind": string}]}`,
		Prompt: fmt.Sprintf("The role-play is over. Give 2 to %d short points in Spanish: what the learner did well "+
			"and what to work on. Then list the learner's errors (only the learner's lines): each fix is the "+
			"smallest wrong chunk, copied exactly as the learner wrote it, its correct version, its Spanish meaning "+
			"and a one-sentence explanation in Spanish. Mark at most %d fixes as major (major true): the errors "+
			"that matter most at this level; the others are minor. The kind of each fix is exactly one of: %s. "+
			"If there are no errors, return an empty fixes list.\n\nTranscript:\n%s",
			MaxPoints, MaxMajors, kindNames(), transcriptText(s, lines)),
	}
	var out Feedback
	if err := ask(ctx, t.models.Conversation, r, &out); err != nil {
		return Feedback{}, err
	}
	var points []string
	for _, p := range out.Points {
		if p = strings.TrimSpace(p); p != "" && len(points) < MaxPoints {
			points = append(points, p)
		}
	}
	if len(points) == 0 {
		return Feedback{}, errors.New("teacher: the role-play feedback came back empty")
	}
	out.Points, out.Fixes = points, cleanFixes(out.Fixes)
	return out, nil
}
