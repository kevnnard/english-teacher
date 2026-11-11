package domain

// Story is a story the learner reads chapter by chapter (extensive
// reading). Only the chapter on screen is kept: the summary carries what
// happened before, so the next chapter can continue the same story.
type Story struct {
	ID      int64  // the store's id; 0 until the story is saved
	Title   string // the same for every chapter
	Chapter int    // the number of the chapter in Text, from 1
	Summary string // the story so far, up to and including this chapter
	Text    string // the chapter itself
	// Details is the chapter's glossary and question as JSON. The screen
	// that shows the story writes and reads it; the store keeps it as is.
	Details string
	Done    bool // the chapter's question is answered: next comes the next chapter
}
