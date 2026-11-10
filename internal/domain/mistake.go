package domain

// Mistake is one major writing error the learner made, kept so the app
// can count recurring kinds of errors across sessions.
type Mistake struct {
	Date  Date   // the day the error was made
	Kind  string // the error kind, such as "be" or "article"
	Wrong string // what the learner wrote
	Right string // the correct chunk
}
