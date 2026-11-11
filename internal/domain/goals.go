package domain

// "Can do" goals (spec S5) are short statements of what a learner at a
// CEFR level can do in English, in the style of the CEFR self-assessment
// grid, paraphrased. Each one names the evidence that proves it: an
// activity the app records. A goal with no such evidence is NotTracked
// and is never ticked.

// GoalRule is the kind of evidence that ticks a goal.
type GoalRule int

const (
	// NotTracked goals have no activity that proves them yet.
	NotTracked GoalRule = iota
	// ActivityDone needs Count finished activities that start with
	// Activity, like "Talk: Introduce yourself".
	ActivityDone
	// ScoreAtLeast needs Count scored activities "<Activity>N/M" with
	// N/M at least Right/Of, like "Listening: 2/3" for Right 2, Of 3.
	ScoreAtLeast
	// ChaptersRead needs Count story chapters read, recorded as
	// "Reading: <title> ch N".
	ChaptersRead
	// StoryAnswered needs one story question answered right. The answer
	// is not in the activity log, so the tick is stored when it happens.
	StoryAnswered
)

// Activity prefixes the goals read; the screens record them.
const (
	TalkPrefix      = "Talk: "
	FluencyPrefix   = "Fluency: "
	ListeningPrefix = "Listening: " // the long listening, "Listening: N/M"
	DictationPrefix = "Listen: "    // a dictation, "Listen: N/M"
	WritingPrefix   = "Writing: "   // a Write rubric, "Writing: N/40"
	GrammarPrefix   = "Quiz: grammar "
	SpeakPrefix     = "Speak: "
	ReadingPrefix   = "Reading: " // a short text, or "Reading: <title> ch N"
)

// Goal is one "can do" statement and the rule that ticks it.
type Goal struct {
	ID    string // stable, like "a1-introduce"; stored ticks use it
	Level Level
	Text  string // "I can …", in English
	Proof string // what to do in the app to prove it, for the learner

	Rule     GoalRule
	Activity string // the activity prefix, for ActivityDone and ScoreAtLeast
	Right    int    // ScoreAtLeast: the lowest share right is Right/Of
	Of       int
	Count    int // how many times; 0 means once
}

// Times is how many matching activities the goal needs, at least 1.
func (g Goal) Times() int {
	return max(g.Count, 1)
}

// Passes reports whether right out of total reaches the goal's share,
// Right/Of. A score with no questions never passes.
func (g Goal) Passes(right, total int) bool {
	return total > 0 && g.Of > 0 && right*g.Of >= g.Right*total
}

// CanDoGoals are the built-in goals of every level, from A1 to C2, in the
// order the Progress screen lists them.
var CanDoGoals = []Goal{
	// A1
	{ID: "a1-introduce", Level: A1, Text: "I can introduce myself and say where I am from.",
		Proof: "finish the Talk role-play “Introduce yourself”", Rule: ActivityDone, Activity: TalkPrefix + "Introduce yourself"},
	{ID: "a1-order", Level: A1, Text: "I can order a drink and ask how much it is.",
		Proof: "finish the Talk role-play “Order a coffee”", Rule: ActivityDone, Activity: TalkPrefix + "Order a coffee"},
	{ID: "a1-directions", Level: A1, Text: "I can ask where a place is and follow a simple answer.",
		Proof: "finish the Talk role-play “Ask for directions”", Rule: ActivityDone, Activity: TalkPrefix + "Ask for directions"},
	{ID: "a1-read", Level: A1, Text: "I can read a short, simple text about everyday things.",
		Proof: "answer a story chapter's question right in Learn (s)", Rule: StoryAnswered},
	{ID: "a1-listen", Level: A1, Text: "I can understand slow, clear speech about familiar things.",
		Proof: "score at least 2/3 in a long listening (Listen, s)", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 2, Of: 3},
	{ID: "a1-write", Level: A1, Text: "I can write short, simple sentences about myself.",
		Proof: "score at least 20/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 20, Of: 40},
	{ID: "a1-form", Level: A1, Text: "I can fill in a simple form with my personal details."},

	// A2
	{ID: "a2-routine", Level: A2, Text: "I can talk about my work and my weekend in simple terms.",
		Proof: "finish the Talk role-play “Small talk at work”", Rule: ActivityDone, Activity: TalkPrefix + "Small talk at work"},
	{ID: "a2-exchanges", Level: A2, Text: "I can handle short social exchanges in shops and on the street.",
		Proof: "finish 3 Talk role-plays", Rule: ActivityDone, Activity: TalkPrefix, Count: 3},
	{ID: "a2-read", Level: A2, Text: "I can read short texts and find the facts I need.",
		Proof: "answer a story chapter's question right in Learn (s)", Rule: StoryAnswered},
	{ID: "a2-listen", Level: A2, Text: "I can catch the main point of short, clear talk.",
		Proof: "score at least 2/3 in a long listening (Listen, s)", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 2, Of: 3},
	{ID: "a2-dictation", Level: A2, Text: "I can write down simple everyday sentences I hear.",
		Proof: "score at least 8/10 in a dictation (Listen)", Rule: ScoreAtLeast, Activity: DictationPrefix, Right: 8, Of: 10},
	{ID: "a2-write", Level: A2, Text: "I can write a short note or message about everyday things.",
		Proof: "score at least 24/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 24, Of: 40},
	{ID: "a2-past", Level: A2, Text: "I can tell a simple story about something that happened to me.",
		Proof: "finish a 4/3/2 session in Fluency", Rule: ActivityDone, Activity: FluencyPrefix},

	// B1
	{ID: "b1-situations", Level: B1, Text: "I can deal with most everyday situations in conversation.",
		Proof: "finish 5 Talk role-plays at this level", Rule: ActivityDone, Activity: TalkPrefix, Count: 5},
	{ID: "b1-story", Level: B1, Text: "I can tell a story or describe an experience and give reasons.",
		Proof: "finish 3 sessions in Fluency", Rule: ActivityDone, Activity: FluencyPrefix, Count: 3},
	{ID: "b1-listen", Level: B1, Text: "I can follow the main points of clear talk on familiar topics.",
		Proof: "score at least 4/5 in a long listening (Listen, s)", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 4, Of: 5},
	{ID: "b1-read", Level: B1, Text: "I can follow a story over several chapters.",
		Proof: "read 5 story chapters in Learn (s)", Rule: ChaptersRead, Count: 5},
	{ID: "b1-write", Level: B1, Text: "I can write a simple connected text on a familiar topic.",
		Proof: "score at least 28/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 28, Of: 40},
	{ID: "b1-grammar", Level: B1, Text: "I can use common grammar correctly in my sentences.",
		Proof: "score at least 80% in a grammar quiz", Rule: ScoreAtLeast, Activity: GrammarPrefix, Right: 4, Of: 5},
	{ID: "b1-opinion", Level: B1, Text: "I can give and explain my opinion briefly in a discussion."},

	// B2
	{ID: "b2-fluent", Level: B2, Text: "I can talk with some fluency and ease on many topics.",
		Proof: "finish 6 sessions in Fluency", Rule: ActivityDone, Activity: FluencyPrefix, Count: 6},
	{ID: "b2-listen", Level: B2, Text: "I can understand the main ideas of longer, complex talk.",
		Proof: "score at least 4/5 in 3 long listenings", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 4, Of: 5, Count: 3},
	{ID: "b2-read", Level: B2, Text: "I can read longer texts and follow how they develop.",
		Proof: "read 10 story chapters in Learn (s)", Rule: ChaptersRead, Count: 10},
	{ID: "b2-write", Level: B2, Text: "I can write a clear, detailed text and argue a point of view.",
		Proof: "score at least 32/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 32, Of: 40},
	{ID: "b2-speak", Level: B2, Text: "I can pronounce English clearly enough to be understood easily.",
		Proof: "score at least 4/5 in Speak", Rule: ScoreAtLeast, Activity: SpeakPrefix, Right: 4, Of: 5},
	{ID: "b2-options", Level: B2, Text: "I can explain the advantages and drawbacks of different options."},

	// C1
	{ID: "c1-fluent", Level: C1, Text: "I can express myself fluently without searching for words.",
		Proof: "finish 10 sessions in Fluency", Rule: ActivityDone, Activity: FluencyPrefix, Count: 10},
	{ID: "c1-listen", Level: C1, Text: "I can understand long talk even when it is loosely organised.",
		Proof: "answer every question of 3 long listenings", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 1, Of: 1, Count: 3},
	{ID: "c1-write", Level: C1, Text: "I can write clear, well-organised texts on complex subjects.",
		Proof: "score at least 36/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 36, Of: 40},
	{ID: "c1-grammar", Level: C1, Text: "I can keep my grammar accurate in context.",
		Proof: "score at least 90% in 3 grammar quizzes", Rule: ScoreAtLeast, Activity: GrammarPrefix, Right: 9, Of: 10, Count: 3},
	{ID: "c1-social", Level: C1, Text: "I can use English flexibly for social and work purposes.",
		Proof: "finish 10 Talk role-plays at this level", Rule: ActivityDone, Activity: TalkPrefix, Count: 10},
	{ID: "c1-implicit", Level: C1, Text: "I can understand long, demanding texts and what they imply."},

	// C2
	{ID: "c2-listen", Level: C2, Text: "I can understand almost anything I hear with ease.",
		Proof: "answer every question of 5 long listenings", Rule: ScoreAtLeast, Activity: ListeningPrefix, Right: 1, Of: 1, Count: 5},
	{ID: "c2-write", Level: C2, Text: "I can write complex texts in a fitting, effective style.",
		Proof: "score at least 38/40 in Write", Rule: ScoreAtLeast, Activity: WritingPrefix, Right: 38, Of: 40},
	{ID: "c2-read", Level: C2, Text: "I can read almost any kind of text with ease.",
		Proof: "read 20 story chapters in Learn (s)", Rule: ChaptersRead, Count: 20},
	{ID: "c2-talk", Level: C2, Text: "I can take part in any conversation with ease.",
		Proof: "finish 15 Talk role-plays at this level", Rule: ActivityDone, Activity: TalkPrefix, Count: 15},
	{ID: "c2-summarise", Level: C2, Text: "I can summarise information from different sources."},
	{ID: "c2-nuance", Level: C2, Text: "I can express fine shades of meaning precisely."},
}

// GoalsFor returns the goals of level, in order; none for an unknown level.
func GoalsFor(level Level) []Goal {
	var goals []Goal
	for _, g := range CanDoGoals {
		if g.Level == level {
			goals = append(goals, g)
		}
	}
	return goals
}
