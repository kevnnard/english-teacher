package progress

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// This file is the adaptive level rule (spec S3): when the learner is
// ready for the next CEFR level, and whether a level check is passed.
//
// The learner is ready when every criterion is met:
//
//   - Known words: at least KnownWordsTarget(level) words have an interval
//     of domain.KnownIntervalDays (21) days or more. The target grows with
//     the level: A1→A2 500, A2→B1 1000, B1→B2 2000, B2→C1 3500, C1→C2 5000.
//   - Writing: the average of the last WritingRounds (5) Write rubrics is
//     at least WritingTarget (32) out of 40.
//   - Grammar quiz: at least GrammarQuizPercent (80%) right over the last
//     GrammarQuizRounds (3) grammar quizzes.
//   - Listen: at least ListeningPercent (85%) right over the last
//     ListeningRounds (3) dictations.
//   - Pairs: at least ListeningPercent (85%) right over the last
//     ListeningRounds (3) minimal-pairs rounds.
//   - Time at level: at least MinDaysAtLevel (21) days since the level
//     started.
//
// A criterion with fewer rounds than it needs is not met, and one with no
// data at all says NoData. C2 is the top level: it has no criteria.

// Thresholds of the readiness rule.
const (
	WritingRounds      = 5  // Write rubrics averaged
	WritingTarget      = 32 // the lowest average, out of WritingMax
	WritingMax         = 40 // the Write rubric's top score
	GrammarQuizRounds  = 3  // grammar quizzes counted
	GrammarQuizPercent = 80 // the lowest share of right answers
	ListeningRounds    = 3  // Listen or Pairs rounds counted, each
	ListeningPercent   = 85 // the lowest share of right answers
	MinDaysAtLevel     = 21 // days since the level started
)

// Pass marks of the level check.
const (
	CheckReadingRight = 2  // out of 3 comprehension questions (2/3)
	CheckReadingOf    = 3  // the questions of the check's reading
	CheckWritingScore = 28 // out of WritingMax
	CheckQuizPercent  = 70 // the lowest share of right answers
)

// NoData is the current value of a criterion with nothing to measure yet.
const NoData = "no data yet"

// Prefixes of the daily-log activities whose scores the rule reads, like
// "Writing: 33/40" or "Quiz: grammar 6/8".
const (
	WritingPrefix     = "Writing: "
	GrammarQuizPrefix = "Quiz: grammar "
	ListenPrefix      = "Listen: "
	PairsPrefix       = "Pairs: "
)

// knownWordsTarget is how many known words each level needs to move up.
var knownWordsTarget = map[domain.Level]int{
	domain.A1: 500, domain.A2: 1000, domain.B1: 2000, domain.B2: 3500, domain.C1: 5000,
}

// KnownWordsTarget is how many known words level needs before the next
// level; 0 for C2 or an unknown level.
func KnownWordsTarget(level domain.Level) int {
	return knownWordsTarget[level]
}

// ReadinessData is the learner's real study data the rule reads.
type ReadinessData struct {
	Cards   []domain.Card // every card; a word counts once, by its recognition card
	Days    []domain.Day  // recorded study days with their activities, in any order
	Started domain.Date   // when the current level started; zero when unknown
}

// Criterion is one condition for the next level and where the learner
// stands on it.
type Criterion struct {
	Name    string // like "Known words"
	Current string // the learner's value, like "312", or NoData
	Target  string // what is needed, like "≥ 500"
	Met     bool
	Tip     string // one short tip in English, for when it is not met
}

// Readiness is how ready the learner is to move from Level to Next.
type Readiness struct {
	Level    domain.Level
	Next     domain.Level // "" at C2, the top level
	Criteria []Criterion  // in a fixed order; none at C2
}

// MetCount is how many criteria are met.
func (r Readiness) MetCount() int {
	n := 0
	for _, c := range r.Criteria {
		if c.Met {
			n++
		}
	}
	return n
}

// Ready reports whether there is a next level and every criterion is met:
// only then is the level check offered.
func (r Readiness) Ready() bool {
	return r.Next != "" && len(r.Criteria) > 0 && r.MetCount() == len(r.Criteria)
}

// ComputeReadiness applies the readiness rule to data for a learner at
// level on today. It is pure: today is passed in.
func ComputeReadiness(data ReadinessData, level domain.Level, today domain.Date) Readiness {
	next, ok := level.Next()
	r := Readiness{Level: level, Next: next}
	if !ok {
		return r
	}
	days := slices.Clone(data.Days)
	slices.SortStableFunc(days, func(a, b domain.Day) int { return compareDates(a.Date, b.Date) })
	r.Criteria = []Criterion{
		knownWords(data.Cards, level),
		writing(ScoresOf(days, WritingPrefix)),
		accuracy("Grammar quiz", ScoresOf(days, GrammarQuizPrefix), GrammarQuizRounds, GrammarQuizPercent,
			"Take a grammar quiz in Quiz."),
		accuracy("Listen", ScoresOf(days, ListenPrefix), ListeningRounds, ListeningPercent,
			"Do a dictation in Listen; replay slowly before you type."),
		accuracy("Pairs", ScoresOf(days, PairsPrefix), ListeningRounds, ListeningPercent,
			"Play a round of Pairs; hear both words after each answer."),
		timeAtLevel(data.Started, today),
	}
	return r
}

func compareDates(a, b domain.Date) int {
	switch {
	case a.Before(b):
		return -1
	case b.Before(a):
		return 1
	}
	return 0
}

func knownWords(cards []domain.Card, level domain.Level) Criterion {
	target := KnownWordsTarget(level)
	c := Criterion{Name: "Known words", Target: fmt.Sprintf("≥ %d", target),
		Tip: "Review every day; a word is known after 21 days."}
	words := map[string]bool{}
	for _, card := range cards {
		if card.Direction == domain.Recognition && card.IntervalDays >= domain.KnownIntervalDays {
			words[strings.ToLower(card.Word)] = true
		}
	}
	if len(cards) == 0 {
		c.Current = NoData
		return c
	}
	c.Current = fmt.Sprint(len(words))
	c.Met = len(words) >= target
	return c
}

func writing(scores []Score) Criterion {
	c := Criterion{Name: "Writing", Target: fmt.Sprintf("avg ≥ %d/%d over %d", WritingTarget, WritingMax, WritingRounds),
		Tip: "Write a text in Write and fix every marked error."}
	last := lastN(scores, WritingRounds)
	if len(last) == 0 {
		c.Current = NoData
		return c
	}
	sum := 0
	for _, s := range last {
		sum += s.Right
	}
	c.Current = fmt.Sprintf("avg %.1f/%d over %d", float64(sum)/float64(len(last)), WritingMax, len(last))
	c.Met = len(last) == WritingRounds && sum >= WritingTarget*WritingRounds
	return c
}

// accuracy is a criterion of the share of right answers over the last
// rounds scores.
func accuracy(name string, scores []Score, rounds, percent int, tip string) Criterion {
	c := Criterion{Name: name, Target: fmt.Sprintf("≥ %d%% over %d", percent, rounds), Tip: tip}
	last := lastN(scores, rounds)
	if len(last) == 0 {
		c.Current = NoData
		return c
	}
	total := sum(last)
	c.Current = fmt.Sprintf("%d%% over %d", total.Right*100/total.Total, len(last))
	c.Met = len(last) == rounds && total.atLeast(percent)
	return c
}

func timeAtLevel(started, today domain.Date) Criterion {
	c := Criterion{Name: "Time at level", Target: fmt.Sprintf("≥ %d days", MinDaysAtLevel),
		Tip: "Keep studying; this one only needs time."}
	if started == (domain.Date{}) {
		c.Current = NoData
		c.Tip = "Run `teacher set level` to start counting."
		return c
	}
	n := max(today.DaysSince(started), 0)
	unit := "days"
	if n == 1 {
		unit = "day"
	}
	c.Current = fmt.Sprintf("%d %s", n, unit)
	c.Met = n >= MinDaysAtLevel
	return c
}

// lastN is the last n scores, or all of them when there are fewer.
func lastN(scores []Score, n int) []Score {
	return scores[max(len(scores)-n, 0):]
}

// Score is a result like 6 right out of 8.
type Score struct {
	Right, Total int
}

// atLeast reports whether s is at least percent right. A score with no
// total is never enough.
func (s Score) atLeast(percent int) bool {
	return s.Total > 0 && s.Right*100 >= percent*s.Total
}

func sum(scores []Score) Score {
	var t Score
	for _, s := range scores {
		t.Right += s.Right
		t.Total += s.Total
	}
	return t
}

// ScoresOf reads the scores of the activities that start with prefix, in
// the order of days and of each day's activities. The rest of the activity
// must be exactly "N/M" with 0 ≤ N ≤ M and M > 0; anything else is skipped.
//
// The daily log stores each distinct activity once per day, so two equal
// results on the same day count once.
func ScoresOf(days []domain.Day, prefix string) []Score {
	scores := []Score{}
	for _, d := range days {
		for _, a := range d.Activities {
			rest, ok := strings.CutPrefix(a, prefix)
			if !ok {
				continue
			}
			if s, ok := parseScore(rest); ok {
				scores = append(scores, s)
			}
		}
	}
	return scores
}

// parseScore reads "N/M" and nothing else.
func parseScore(s string) (Score, bool) {
	var sc Score
	if _, err := fmt.Sscanf(s, "%d/%d", &sc.Right, &sc.Total); err != nil {
		return Score{}, false
	}
	if fmt.Sprintf("%d/%d", sc.Right, sc.Total) != s || sc.Total <= 0 || sc.Right < 0 || sc.Right > sc.Total {
		return Score{}, false
	}
	return sc, true
}

// LevelCheck is the result of the three parts of a level check.
type LevelCheck struct {
	Reading Score // comprehension questions right of the check's reading
	Writing Score // the Write rubric of the check's text, out of WritingMax
	Quiz    Score // the grammar quiz at the next level
}

// Failed names the parts below their pass mark, in order: "reading"
// below 2/3, "writing" below 28/40 and "grammar quiz" below 70%.
func (c LevelCheck) Failed() []string {
	failed := []string{}
	if c.Reading.Total == 0 || c.Reading.Right*CheckReadingOf < CheckReadingRight*c.Reading.Total {
		failed = append(failed, "reading")
	}
	if c.Writing.Total == 0 || c.Writing.Right*WritingMax < CheckWritingScore*c.Writing.Total {
		failed = append(failed, "writing")
	}
	if !c.Quiz.atLeast(CheckQuizPercent) {
		failed = append(failed, "grammar quiz")
	}
	return failed
}

// Passed reports whether every part reached its pass mark.
func (c LevelCheck) Passed() bool {
	return len(c.Failed()) == 0
}
