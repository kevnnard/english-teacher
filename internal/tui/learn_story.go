package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// Stories (spec S1, extensive reading) live inside Learn: s switches
// between short texts and a story read chapter by chapter. Each chapter
// goes text → glossary → one question; answering records the chapter and
// its words, and n asks for the next chapter of the same story.

// SaveStoryFunc stores the story being read and returns its id, like
// store.SaveStory: ID 0 means a new story.
type SaveStoryFunc func(domain.Story) (int64, error)

// maxChapterMinutes caps the study time one chapter can record, like
// maxReadingMinutes for a short text; a chapter is about four times longer.
const maxChapterMinutes = 30

type storyPhase int

const (
	storyIdle     storyPhase = iota // not opened since the app started
	storyLoading                    // waiting for the teacher
	storyFailed                     // no chapter: the teacher failed or the saved one is damaged
	storyReading                    // the chapter's text, scrolled with ↑↓
	storyGlossary                   // the chapter's glossary, to mark words
	storyQuestion                   // the comprehension question
	storyAnswered                   // the answer is shown and the chapter recorded
)

// storyState is the story half of Learn.
type storyState struct {
	on      bool // Learn shows the story instead of the short texts
	phase   storyPhase
	req     int                  // the id of the latest request; older answers are ignored
	saved   domain.Story         // the story as stored: title, chapter, summary, text
	chapter teacher.StoryChapter // the glossary and question of the chapter on screen
	scroll  int                  // the first text line on screen
	marked  []bool
	cursor  int
	choice  int // the option chosen, from 0
	shownAt time.Time
	err     error // why the last chapter failed or could not be saved
}

// errDamagedChapter means the saved chapter has no glossary or question
// that can be read back, so the learner goes on to the next chapter.
var errDamagedChapter = errors.New("could not read the saved chapter")

// storyDetails is what domain.Story.Details holds: the parts of a
// chapter that only this screen uses.
type storyDetails struct {
	Glossary []teacher.Gloss  `json:"glossary"`
	Question teacher.QuizItem `json:"question"`
}

// chapterMsg is the teacher's answer to request req, which asked for the
// chapter after so.
type chapterMsg struct {
	req     int
	so      teacher.StorySoFar
	chapter teacher.StoryChapter
	err     error
}

// finished reports whether the saved story has no chapters left.
func (s storyState) finished() bool {
	return s.saved.Done && s.saved.Chapter >= teacher.StoryChapters
}

// toggleStory switches Learn between short texts and the story. The
// first time, it opens the saved chapter or asks for the next one.
func (a App) toggleStory() (App, tea.Cmd) {
	if a.teacher == nil {
		return a, nil
	}
	s := &a.story
	s.on = !s.on
	if !s.on || s.phase != storyIdle {
		return a, nil
	}
	if s.saved.Title == "" || s.saved.Done {
		return a.askChapter()
	}
	var d storyDetails
	if err := json.Unmarshal([]byte(s.saved.Details), &d); err != nil || len(d.Question.Options) != teacher.StoryOptions {
		// Nothing to ask about, so the chapter counts as read: enter goes on.
		s.saved.Done = true
		s.phase, s.err = storyFailed, errDamagedChapter
		return a, nil
	}
	a.showChapter(teacher.StoryChapter{
		Title: s.saved.Title, Text: s.saved.Text, Summary: s.saved.Summary,
		Glossary: d.Glossary, Question: d.Question,
	})
	return a, nil
}

// askChapter asks for the chapter after the saved one, or for the first
// chapter of a new story when there is none or it has ended.
func (a App) askChapter() (App, tea.Cmd) {
	s := &a.story
	var so teacher.StorySoFar
	if s.saved.Title != "" && !s.finished() {
		so = teacher.StorySoFar{Title: s.saved.Title, Chapter: s.saved.Chapter, Summary: s.saved.Summary}
	}
	s.req++
	s.phase, s.err = storyLoading, nil
	req, tc, level, known := s.req, a.teacher, a.profile.Level, a.knownWords()
	return a, func() tea.Msg {
		ch, err := tc.StoryChapter(context.Background(), level, known, so)
		return chapterMsg{req: req, so: so, chapter: ch, err: err}
	}
}

// gotChapter saves and shows the answer to the latest request.
func (a App) gotChapter(msg chapterMsg) App {
	s := &a.story
	if msg.req != s.req || s.phase != storyLoading {
		return a
	}
	if msg.err != nil {
		s.phase, s.err = storyFailed, msg.err
		return a
	}
	details, _ := json.Marshal(storyDetails{Glossary: msg.chapter.Glossary, Question: msg.chapter.Question})
	next := domain.Story{
		Title: msg.chapter.Title, Chapter: msg.so.Chapter + 1, Summary: msg.chapter.Summary,
		Text: msg.chapter.Text, Details: string(details),
	}
	if msg.so.Title != "" {
		next.ID = s.saved.ID // the same story keeps its row
	}
	s.saved = next
	a.saveStory()
	a.showChapter(msg.chapter)
	return a
}

// showChapter puts ch on screen from the top of its text.
func (a *App) showChapter(ch teacher.StoryChapter) {
	s := &a.story
	s.chapter, s.phase, s.scroll, s.cursor, s.shownAt = ch, storyReading, 0, 0, a.now()
	s.marked = make([]bool, len(ch.Glossary))
	for i, g := range ch.Glossary {
		s.marked[i] = g.New
	}
}

// saveStory stores the story; a failure is shown but the reading goes on.
func (a *App) saveStory() {
	if a.saveStoryFn == nil {
		return
	}
	id, err := a.saveStoryFn(a.story.saved)
	if err != nil {
		a.story.err = fmt.Errorf("could not save the story: %w", err)
		return
	}
	a.story.saved.ID = id
}

func (a App) updateStory(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &a.story
	k := msg.String()
	switch s.phase {
	case storyFailed:
		if k == "enter" {
			return a.askChapter()
		}
	case storyReading:
		switch k {
		case "down", "j":
			s.scroll++ // the view stops it at the last page
		case "up", "k":
			s.scroll = max(s.scroll-1, 0)
		case "enter":
			s.phase = storyGlossary
		}
	case storyGlossary:
		switch k {
		case "down", "j":
			s.cursor = min(s.cursor+1, max(len(s.marked)-1, 0))
		case "up", "k":
			s.cursor = max(s.cursor-1, 0)
		case "space":
			if len(s.marked) > 0 {
				s.marked[s.cursor] = !s.marked[s.cursor]
			}
		case "a":
			if _, err := a.addGlossary(s.chapter.Glossary, s.marked); err != nil {
				s.err = fmt.Errorf("could not add the words: %w", err)
				return a, nil
			}
			s.phase, s.err = storyQuestion, nil
		}
	case storyQuestion:
		if len(k) == 1 && k[0] >= '1' && int(k[0]-'1') < teacher.StoryOptions {
			a.answerChapter(int(k[0] - '1'))
		}
	case storyAnswered:
		if k == "n" {
			return a.askChapter()
		}
	}
	return a, nil
}

// answerChapter shows whether choice is right and records the chapter:
// its study minutes, its words and "Reading: <title> ch N".
func (a *App) answerChapter(choice int) {
	s := &a.story
	s.choice, s.phase = choice, storyAnswered
	minutes := min(int(a.now().Sub(s.shownAt)/time.Minute), maxChapterMinutes)
	a.addToToday(domain.Day{
		Minutes:    max(minutes, 0),
		WordsRead:  progress.CountWords(s.saved.Text),
		Activities: []string{fmt.Sprintf("Reading: %s ch %d", s.saved.Title, s.saved.Chapter)},
	})
	s.saved.Done = true
	a.saveStory()
}

// wordsReadThisWeek is the stored count of words read this week.
func (a App) wordsReadThisWeek() int {
	return progress.WordsReadThisWeek(a.days, a.today())
}
