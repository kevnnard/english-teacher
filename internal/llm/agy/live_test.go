package agy

import (
	"context"
	"os"
	"testing"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

func TestLiveComplete(t *testing.T) {
	if os.Getenv("TEACHER_LIVE_AGY") != "1" {
		t.Skip("set TEACHER_LIVE_AGY=1 to call the real agy")
	}
	ids, err := ListModels(context.Background(), "")
	if err != nil || len(ids) == 0 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	out, err := Model{Model: ids[len(ids)-1]}.Complete(context.Background(), teacher.Request{System: "Answer with one word.", Prompt: "Reply with exactly: OK"})
	if err != nil || out == "" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	t.Logf("%s: %s", ids[len(ids)-1], out)
}
