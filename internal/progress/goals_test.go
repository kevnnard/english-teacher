package progress

import (
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// goalStatus finds the status of goal id, or fails.
func goalStatus(t *testing.T, all []GoalStatus, id string) GoalStatus {
	t.Helper()
	for _, g := range all {
		if g.Goal.ID == id {
			return g
		}
	}
	t.Fatalf("no goal %q", id)
	return GoalStatus{}
}

func act(d domain.Date, activities ...string) domain.Day {
	return domain.Day{Date: d, Activities: activities}
}

func TestEachGoalRuleTicksOnlyOnEvidence(t *testing.T) {
	tests := []struct {
		name  string
		level domain.Level
		id    string
		days  []domain.Day
		ticks map[string]domain.Date
		want  bool
		on    domain.Date
	}{
		{"talk scenario done", domain.A1, "a1-introduce", []domain.Day{act(mon9, "Talk: Introduce yourself")}, nil, true, mon9},
		{"another scenario does not count", domain.A1, "a1-introduce", []domain.Day{act(mon9, "Talk: Order a coffee")}, nil, false, domain.Date{}},
		{"long listening 2/3", domain.A1, "a1-listen", []domain.Day{act(mon2, "Listening: 1/3"), act(sun8, "Listening: 2/3")}, nil, true, sun8},
		{"long listening 3/5 is below 2/3", domain.A1, "a1-listen", []domain.Day{act(mon2, "Listening: 3/5")}, nil, false, domain.Date{}},
		{"a dictation is not a long listening", domain.A1, "a1-listen", []domain.Day{act(mon2, "Listen: 10/10")}, nil, false, domain.Date{}},
		{"write 20/40", domain.A1, "a1-write", []domain.Day{act(mon9, "Writing: 20/40")}, nil, true, mon9},
		{"write 19/40", domain.A1, "a1-write", []domain.Day{act(mon9, "Writing: 19/40")}, nil, false, domain.Date{}},
		{"story answered: stored tick", domain.A1, "a1-read", nil, map[string]domain.Date{"a1-read": sun8}, true, sun8},
		{"story answered: a chapter alone is not enough", domain.A1, "a1-read", []domain.Day{act(mon9, "Reading: The key ch 1")}, nil, false, domain.Date{}},
		{"three talks, on the third", domain.A2, "a2-exchanges",
			[]domain.Day{act(mon2, "Talk: Order a coffee", "Talk: Ask for directions"), act(mon9, "Talk: Order a coffee")}, nil, true, mon9},
		{"two talks", domain.A2, "a2-exchanges", []domain.Day{act(mon2, "Talk: Order a coffee", "Talk: Ask for directions")}, nil, false, domain.Date{}},
		{"five chapters", domain.B1, "b1-read", []domain.Day{
			act(mon2, "Reading: The key ch 1", "Reading: The key ch 2", "Reading: Market"),
			act(sun8, "Reading: The key ch 3", "Reading: The key ch 4"), act(mon9, "Reading: The key ch 5")}, nil, true, mon9},
		{"a short text is not a chapter", domain.B1, "b1-read", []domain.Day{
			act(mon2, "Reading: The key ch 1", "Reading: The key ch 2", "Reading: Market", "Reading: The key ch 3", "Reading: The key ch 4")}, nil, false, domain.Date{}},
		{"score goal needs Count passing rounds", domain.B2, "b2-listen", []domain.Day{
			act(mon2, "Listening: 4/5"), act(sun8, "Listening: 3/5"), act(mon9, "Listening: 5/5", "Listening: 4/4")}, nil, true, mon9},
	}
	for _, tt := range tests {
		got := goalStatus(t, CanDo(tt.level, GoalData{Days: tt.days, Ticks: tt.ticks}), tt.id)
		if got.Ticked != tt.want || got.On != tt.on {
			t.Errorf("%s: Ticked %v on %v, want %v on %v", tt.name, got.Ticked, got.On, tt.want, tt.on)
		}
	}
}

func TestGoalsOnlyCountEvidenceSinceTheLevelStarted(t *testing.T) {
	days := []domain.Day{act(mon2, "Talk: Introduce yourself")}
	if g := goalStatus(t, CanDo(domain.A1, GoalData{Days: days, Since: mon9}), "a1-introduce"); g.Ticked {
		t.Error("a talk before the level started ticks the goal")
	}
	if g := goalStatus(t, CanDo(domain.A1, GoalData{Days: days, Since: mon2}), "a1-introduce"); !g.Ticked {
		t.Error("a talk on the day the level started does not tick the goal")
	}
	// A stored tick is evidence of its own and keeps its date.
	ticks := map[string]domain.Date{"a1-introduce": sun1}
	if g := goalStatus(t, CanDo(domain.A1, GoalData{Ticks: ticks, Since: mon9}), "a1-introduce"); !g.Ticked || g.On != sun1 {
		t.Errorf("stored tick: %+v", g)
	}
}

func TestUntrackedGoalsAreNeverTicked(t *testing.T) {
	all := CanDo(domain.A1, GoalData{Ticks: map[string]domain.Date{"a1-form": mon9}})
	g := goalStatus(t, all, "a1-form")
	if g.Tracked() || g.Ticked {
		t.Errorf("a1-form: %+v, want not tracked and not ticked", g)
	}
	if len(all) != len(domain.GoalsFor(domain.A1)) {
		t.Errorf("%d statuses, want one per A1 goal", len(all))
	}
	if n := TickedCount(all); n != 0 {
		t.Errorf("TickedCount = %d, want 0", n)
	}
}
