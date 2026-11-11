package progress

import (
	"fmt"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// knownCards returns n recognition cards with the given interval, plus a
// production card of each word, which must not count twice.
func knownCards(n, interval int) []domain.Card {
	var cards []domain.Card
	for i := range n {
		c := domain.NewCard(fmt.Sprintf("word%d", i))
		c.IntervalDays = interval
		p := c
		p.Direction = domain.Production
		cards = append(cards, c, p)
	}
	return cards
}

// logged returns one day per activity, oldest first, ending today.
func logged(acts ...string) []domain.Day {
	days := make([]domain.Day, len(acts))
	for i, a := range acts {
		days[i] = domain.Day{Date: today.AddDays(i - len(acts) + 1), Activities: []string{a}}
	}
	return days
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// criterion returns the criterion called name.
func criterion(t *testing.T, r Readiness, name string) Criterion {
	t.Helper()
	for _, c := range r.Criteria {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no criterion %q in %+v", name, r.Criteria)
	return Criterion{}
}

func TestReadinessCriteria(t *testing.T) {
	tests := []struct {
		name    string
		data    ReadinessData
		level   domain.Level
		crit    string
		current string
		target  string
		met     bool
	}{
		// Known words: interval ≥ 21 days, one per word, target scaled by level.
		{"known: none", ReadinessData{}, domain.A1, "Known words", NoData, "≥ 500", false},
		{"known: 499", ReadinessData{Cards: knownCards(499, 21)}, domain.A1, "Known words", "499", "≥ 500", false},
		{"known: 500", ReadinessData{Cards: knownCards(500, 21)}, domain.A1, "Known words", "500", "≥ 500", true},
		{"known: interval 20 is not known", ReadinessData{Cards: knownCards(500, 20)}, domain.A1, "Known words", "0", "≥ 500", false},
		{"known: A2 needs 1000", ReadinessData{Cards: knownCards(999, 30)}, domain.A2, "Known words", "999", "≥ 1000", false},
		{"known: B1 needs 2000", ReadinessData{Cards: knownCards(2000, 30)}, domain.B1, "Known words", "2000", "≥ 2000", true},
		{"known: B2 needs 3500", ReadinessData{Cards: knownCards(10, 30)}, domain.B2, "Known words", "10", "≥ 3500", false},
		{"known: C1 needs 5000", ReadinessData{Cards: knownCards(10, 30)}, domain.C1, "Known words", "10", "≥ 5000", false},

		// Writing: average of the last 5 ≥ 32/40.
		{"writing: none", ReadinessData{}, domain.A1, "Writing", NoData, "avg ≥ 32/40 over 5", false},
		{"writing: fewer than 5", ReadinessData{Days: logged(repeat("Writing: 40/40", 4)...)}, domain.A1,
			"Writing", "avg 40.0/40 over 4", "avg ≥ 32/40 over 5", false},
		{"writing: just below", ReadinessData{Days: logged("Writing: 32/40", "Writing: 32/40", "Writing: 32/40", "Writing: 32/40", "Writing: 31/40")},
			domain.A1, "Writing", "avg 31.8/40 over 5", "avg ≥ 32/40 over 5", false},
		{"writing: at 32", ReadinessData{Days: logged(repeat("Writing: 32/40", 5)...)}, domain.A1,
			"Writing", "avg 32.0/40 over 5", "avg ≥ 32/40 over 5", true},
		{"writing: only the last 5 count", ReadinessData{Days: logged(append([]string{"Writing: 0/40"}, repeat("Writing: 34/40", 5)...)...)},
			domain.A1, "Writing", "avg 34.0/40 over 5", "avg ≥ 32/40 over 5", true},

		// Grammar quiz: ≥ 80% over the last 3.
		{"quiz: none", ReadinessData{Days: logged("Quiz: vocabulary 8/8")}, domain.A1, "Grammar quiz", NoData, "≥ 80% over 3", false},
		{"quiz: just below", ReadinessData{Days: logged("Quiz: grammar 8/10", "Quiz: grammar 8/10", "Quiz: grammar 7/10")},
			domain.A1, "Grammar quiz", "76% over 3", "≥ 80% over 3", false},
		{"quiz: at 80", ReadinessData{Days: logged("Quiz: grammar 8/10", "Quiz: grammar 8/10", "Quiz: grammar 8/10")},
			domain.A1, "Grammar quiz", "80% over 3", "≥ 80% over 3", true},
		{"quiz: fewer than 3", ReadinessData{Days: logged("Quiz: grammar 8/8", "Quiz: grammar 8/8")},
			domain.A1, "Grammar quiz", "100% over 2", "≥ 80% over 3", false},

		// Listen and Pairs: ≥ 85% over the last 3 rounds each.
		{"listen: none", ReadinessData{}, domain.A1, "Listen", NoData, "≥ 85% over 3", false},
		{"listen: just below", ReadinessData{Days: logged("Listen: 9/10", "Listen: 8/10", "Listen: 8/10")},
			domain.A1, "Listen", "83% over 3", "≥ 85% over 3", false},
		{"listen: at 85", ReadinessData{Days: logged("Listen: 9/10", "Listen: 9/10", "Listen: 8/10", "Listen: 9/10")},
			domain.A1, "Listen", "86% over 3", "≥ 85% over 3", true},
		{"pairs: none", ReadinessData{Days: logged("Listening: 5/5")}, domain.A1, "Pairs", NoData, "≥ 85% over 3", false},
		{"pairs: just below", ReadinessData{Days: logged("Pairs: 17/20", "Pairs: 17/20", "Pairs: 16/20")},
			domain.A1, "Pairs", "83% over 3", "≥ 85% over 3", false},
		{"pairs: at 85", ReadinessData{Days: logged("Pairs: 17/20", "Pairs: 17/20", "Pairs: 17/20")},
			domain.A1, "Pairs", "85% over 3", "≥ 85% over 3", true},

		// Time: ≥ 21 days since the level started.
		{"time: unknown start", ReadinessData{}, domain.A1, "Time at level", NoData, "≥ 21 days", false},
		{"time: 20 days", ReadinessData{Started: today.AddDays(-20)}, domain.A1, "Time at level", "20 days", "≥ 21 days", false},
		{"time: 21 days", ReadinessData{Started: today.AddDays(-21)}, domain.A1, "Time at level", "21 days", "≥ 21 days", true},
		{"time: 1 day", ReadinessData{Started: today.AddDays(-1)}, domain.A1, "Time at level", "1 day", "≥ 21 days", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := criterion(t, ComputeReadiness(tt.data, tt.level, today), tt.crit)
			if c.Current != tt.current || c.Target != tt.target || c.Met != tt.met {
				t.Errorf("%s = %q / %q met %v; want %q / %q met %v",
					tt.crit, c.Current, c.Target, c.Met, tt.current, tt.target, tt.met)
			}
			if c.Tip == "" {
				t.Errorf("%s has no tip", tt.crit)
			}
		})
	}
}

func TestReadinessOrderAndCount(t *testing.T) {
	r := ComputeReadiness(ReadinessData{Started: today.AddDays(-30)}, domain.A1, today)
	if r.Level != domain.A1 || r.Next != domain.A2 {
		t.Errorf("levels = %v → %v", r.Level, r.Next)
	}
	var names []string
	for _, c := range r.Criteria {
		names = append(names, c.Name)
	}
	want := "[Known words Writing Grammar quiz Listen Pairs Time at level]"
	if fmt.Sprint(names) != want {
		t.Errorf("names = %v, want %s", names, want)
	}
	if r.MetCount() != 1 || r.Ready() {
		t.Errorf("met = %d, ready = %v; want 1, false", r.MetCount(), r.Ready())
	}
}

// readyData meets every A1 criterion.
func readyData() ReadinessData {
	var acts []string
	acts = append(acts, repeat("Writing: 33/40", 5)...)
	acts = append(acts, repeat("Quiz: grammar 7/8", 3)...)
	acts = append(acts, repeat("Listen: 9/10", 3)...)
	acts = append(acts, repeat("Pairs: 18/20", 3)...)
	return ReadinessData{Cards: knownCards(500, 21), Days: logged(acts...), Started: today.AddDays(-21)}
}

func TestReadinessReadyWhenEveryCriterionIsMet(t *testing.T) {
	r := ComputeReadiness(readyData(), domain.A1, today)
	if !r.Ready() || r.MetCount() != len(r.Criteria) {
		t.Errorf("ready = %v, met %d of %d: %+v", r.Ready(), r.MetCount(), len(r.Criteria), r.Criteria)
	}
}

func TestReadinessAtC2HasNoNextLevel(t *testing.T) {
	r := ComputeReadiness(readyData(), domain.C2, today)
	if r.Next != "" || len(r.Criteria) != 0 || r.Ready() {
		t.Errorf("C2 readiness = %+v", r)
	}
}

func TestReadinessReadsDaysInDateOrder(t *testing.T) {
	// The newest five writings decide, even when the days come unsorted.
	days := logged(append([]string{"Writing: 0/40"}, repeat("Writing: 40/40", 5)...)...)
	days[0], days[5] = days[5], days[0]
	c := criterion(t, ComputeReadiness(ReadinessData{Days: days}, domain.A1, today), "Writing")
	if c.Current != "avg 40.0/40 over 5" {
		t.Errorf("current = %q", c.Current)
	}
}

func TestScoresOf(t *testing.T) {
	days := []domain.Day{
		{Date: today.AddDays(-1), Activities: []string{"Writing: 30/40", "Quiz: grammar 6/8", "Reading: A day"}},
		{Date: today, Activities: []string{"Quiz: vocabulary 5/5", "Writing: 35/40", "Writing: x/40", "Writing: 50/40", "Listen: 9/10", "Pairs: 18/20 extra"}},
	}
	tests := []struct {
		prefix string
		want   string
	}{
		{WritingPrefix, "[{30 40} {35 40}]"},
		{GrammarQuizPrefix, "[{6 8}]"},
		{ListenPrefix, "[{9 10}]"},
		{PairsPrefix, "[]"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(ScoresOf(days, tt.prefix)); got != tt.want {
			t.Errorf("ScoresOf(%q) = %s, want %s", tt.prefix, got, tt.want)
		}
	}
}

func TestLevelCheckPassMarks(t *testing.T) {
	tests := []struct {
		name   string
		check  LevelCheck
		failed string
	}{
		{"all at the bar", LevelCheck{Reading: Score{2, 3}, Writing: Score{28, 40}, Quiz: Score{7, 10}}, "[]"},
		{"reading below", LevelCheck{Reading: Score{1, 3}, Writing: Score{28, 40}, Quiz: Score{7, 10}}, "[reading]"},
		{"writing below", LevelCheck{Reading: Score{3, 3}, Writing: Score{27, 40}, Quiz: Score{8, 8}}, "[writing]"},
		{"quiz below", LevelCheck{Reading: Score{3, 3}, Writing: Score{40, 40}, Quiz: Score{5, 8}}, "[grammar quiz]"},
		{"nothing done", LevelCheck{}, "[reading writing grammar quiz]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(tt.check.Failed())
		if got != tt.failed {
			t.Errorf("%s: failed = %s, want %s", tt.name, got, tt.failed)
		}
		if tt.check.Passed() != (tt.failed == "[]") {
			t.Errorf("%s: passed = %v", tt.name, tt.check.Passed())
		}
	}
}
