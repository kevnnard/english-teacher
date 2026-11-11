package domain

import "testing"

func TestEveryLevelHasFiveToEightGoals(t *testing.T) {
	ids := map[string]bool{}
	for _, l := range Levels {
		goals := GoalsFor(l)
		if len(goals) < 5 || len(goals) > 8 {
			t.Errorf("%v has %d goals, want 5–8", l, len(goals))
		}
		for _, g := range goals {
			if ids[g.ID] {
				t.Errorf("goal id %q is used twice", g.ID)
			}
			ids[g.ID] = true
			if len(g.Text) < 8 || g.Text[:6] != "I can " {
				t.Errorf("%s: text %q does not start with \"I can \"", g.ID, g.Text)
			}
			if g.Rule != NotTracked && g.Proof == "" {
				t.Errorf("%s: a tracked goal needs a proof", g.ID)
			}
			if g.Rule == ScoreAtLeast && (g.Of <= 0 || g.Right <= 0 || g.Right > g.Of || g.Activity == "") {
				t.Errorf("%s: a score goal needs Activity and 0 < Right ≤ Of", g.ID)
			}
			if g.Rule == ActivityDone && g.Activity == "" {
				t.Errorf("%s: an activity goal needs Activity", g.ID)
			}
		}
	}
	if GoalsFor("X9") != nil {
		t.Error("an unknown level has goals")
	}
}

func TestGoalPasses(t *testing.T) {
	g := Goal{Right: 2, Of: 3}
	for _, tt := range []struct {
		right, total int
		want         bool
	}{{2, 3, true}, {3, 3, true}, {4, 5, true}, {3, 5, false}, {1, 3, false}, {0, 0, false}} {
		if got := g.Passes(tt.right, tt.total); got != tt.want {
			t.Errorf("Passes(%d, %d) = %v, want %v", tt.right, tt.total, got, tt.want)
		}
	}
	if (Goal{}).Times() != 1 || (Goal{Count: 3}).Times() != 3 {
		t.Error("Times: want 1 by default and Count otherwise")
	}
}
