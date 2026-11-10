// Package teacher holds the AI teacher's use cases: it writes the prompts,
// asks a Model and turns the answer into domain values. It knows nothing
// about which AI runs behind Model; adapters such as internal/llm/pi do.
package teacher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Request is one question to a model.
type Request struct {
	System string // who the model is and the answer format
	Prompt string // the question itself
}

// Model is any AI that answers a request with text.
type Model interface {
	Complete(ctx context.Context, r Request) (string, error)
}

// Models is the model for each capability, so each job can use the model
// that does it best. A nil model means the capability is off.
type Models struct {
	Reading      Model // graded readings
	Writing      Model // writing tasks and corrections
	Conversation Model // role-plays
	Quiz         Model // quiz questions
}

// ErrNoModel means no model is configured for a capability.
var ErrNoModel = errors.New("no model configured")

// Teacher runs the AI use cases.
type Teacher struct {
	models Models
}

// New returns a teacher that uses models.
func New(models Models) *Teacher {
	return &Teacher{models: models}
}

// learner describes who the model is teaching, for every system prompt.
func learner(level domain.Level) string {
	return fmt.Sprintf("You are a kind, precise English teacher. Your learner speaks Rioplatense Spanish "+
		"and is at CEFR level %s (%s). Explanations for the learner are in Spanish (voseo); "+
		"English examples are natural, correct and at the learner's level.", level, level.Name())
}

// jsonOnly is appended to every system prompt that expects JSON back.
const jsonOnly = " Reply with ONLY one JSON object, no markdown and no text around it, exactly in this shape: "

// ask sends the request to m and decodes the JSON answer into v.
func ask(ctx context.Context, m Model, r Request, v any) error {
	if m == nil {
		return ErrNoModel
	}
	out, err := m.Complete(ctx, r)
	if err != nil {
		return fmt.Errorf("teacher: %w", err)
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), v); err != nil {
		return fmt.Errorf("teacher: the answer is not valid JSON (%v): %.120q", err, out)
	}
	return nil
}

// extractJSON returns the outermost {...} of s, which drops markdown
// fences or a sentence a model may add despite being told not to.
func extractJSON(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

// list joins words for a prompt, or says there are none.
func list(words []string) string {
	if len(words) == 0 {
		return "(none yet)"
	}
	return strings.Join(words, ", ")
}
