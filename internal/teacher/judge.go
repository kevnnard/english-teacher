package teacher

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Attempt is the learner's try at fixing one marked error alone.
type Attempt struct {
	Sentence string // the learner's sentence with the error
	Wrong    string // the chunk the learner wrote first
	Right    string // the teacher's correction
	Attempt  string // what the learner typed to fix it
}

// Judge says, for each attempt, whether it fixes the error. A valid
// alternative to Right counts as fixed. Every attempt goes in one call,
// and the answer has exactly one verdict per attempt, in order.
func (t *Teacher) Judge(ctx context.Context, level domain.Level, attempts []Attempt) ([]bool, error) {
	if len(attempts) == 0 {
		return nil, nil
	}
	var b strings.Builder
	for i, a := range attempts {
		fmt.Fprintf(&b, "%d. Sentence: %q\n   Error: %q\n   Teacher's correction: %q\n   Learner's fix: %q\n",
			i+1, a.Sentence, a.Wrong, a.Right, a.Attempt)
	}
	r := Request{
		System: learner(level) + jsonOnly + `{"fixed": [boolean]}`,
		Prompt: fmt.Sprintf("The learner tried to fix each marked error alone. For each numbered item say whether "+
			"the learner's fix is correct English that keeps the meaning: the teacher's correction or any valid "+
			"alternative counts as fixed (true); a fix that is still wrong is false. Ignore case and final "+
			"punctuation. Answer with exactly %d booleans, in order.\n\n%s", len(attempts), b.String()),
	}
	var out struct {
		Fixed []bool `json:"fixed"`
	}
	if err := ask(ctx, t.models.Writing, r, &out); err != nil {
		return nil, err
	}
	if len(out.Fixed) != len(attempts) {
		return nil, fmt.Errorf("teacher: the judge answered %d verdicts for %d attempts", len(out.Fixed), len(attempts))
	}
	return out.Fixed, nil
}
