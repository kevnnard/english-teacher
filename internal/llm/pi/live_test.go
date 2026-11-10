package pi

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// TestLiveCorrection asks the real model through pi. It only runs with
// TEACHER_LIVE=1, so the normal test suite never needs the network.
func TestLiveCorrection(t *testing.T) {
	if os.Getenv("TEACHER_LIVE") != "1" {
		t.Skip("set TEACHER_LIVE=1 to call the real model")
	}
	m := Model{Model: "claude-bridge/claude-sonnet-5-5"}
	tc := teacher.New(teacher.Models{Writing: m, Reading: m})

	c, err := tc.Correct(context.Background(), domain.A1, "Introduce yourself", "I has 30 years. I am from Argentina.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("correction: %+v", c)
	if !strings.Contains(c.Corrected, "30 years old") || len(c.Fixes) == 0 {
		t.Errorf("the age error was not corrected: %+v", c)
	}

	task, err := tc.WritingTask(context.Background(), domain.A1, []string{"brother", "I'm from"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task: %+v", task)

	r, err := tc.Reading(context.Background(), domain.A1, []string{"hello", "name", "Argentina", "teacher"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reading: %s\n%s\nglossary: %d", r.Title, r.Text, len(r.Glossary))
	news := 0
	for _, g := range r.Glossary {
		if g.New {
			news++
		}
	}
	if news > teacher.MaxNewWords {
		t.Errorf("%d new words, want at most %d", news, teacher.MaxNewWords)
	}
}
