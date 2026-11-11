package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// The Level screen shows how ready the learner is for the next CEFR level
// (spec S3) and, once every criterion is met, runs the level check. The
// check is a guided sequence over the existing screens:
//
//  1. a reading with comprehension questions, on the Level screen itself;
//  2. a short text on the Write screen, scored with its rubric;
//  3. a grammar quiz on the Quiz screen.
//
// While parts 2 and 3 run, studyLevel is the next level, so Write and Quiz
// ask the teacher for that level. Their results come back through
// checkWritten and checkQuizzed, and the learner returns to this screen.

// checkStage is where a level check stands.
type checkStage int

const (
	checkOff         checkStage = iota // no check is running
	checkReadingWait                   // waiting for the reading
	checkReadingFail                   // the reading could not be written
	checkReading                       // the reading questions are being answered
	checkReadingDone                   // the reading is scored; the writing comes next
	checkWriting                       // the Write screen runs at the next level
	checkQuizReady                     // the writing is scored; the quiz comes next
	checkQuiz                          // the Quiz screen runs at the next level
	checkDone                          // the result is on screen
)

// levelCheckState is one level check: its parts, scores and result.
type levelCheckState struct {
	stage   checkStage
	next    domain.Level // the level being checked
	from    domain.Level // the level the learner had when the check started
	req     int          // the id of the latest reading request
	reading teacher.CheckReading
	picked  []int // the options chosen, one per answered question
	result  progress.LevelCheck
	passed  bool
	show    bool  // the result waits to be shown when the quiz is left
	err     error // the reading request or the profile save failed
}

// checkReadingMsg is the teacher's answer to reading request req.
type checkReadingMsg struct {
	req     int
	reading teacher.CheckReading
	err     error
}

// readiness applies the readiness rule to the learner's data today.
func (a App) readiness() progress.Readiness {
	data := progress.ReadinessData{Cards: a.cards, Days: a.days, Started: a.profile.Started}
	return progress.ComputeReadiness(data, a.profile.Level, a.today())
}

// studyLevel is the level the AI teacher writes for: the next level while
// the writing or the quiz of a level check runs, else the learner's level.
func (a App) studyLevel() domain.Level {
	if a.check.stage == checkWriting || a.check.stage == checkQuiz {
		return a.check.next
	}
	return a.profile.Level
}

// running reports whether a level check has started and not ended.
func (c levelCheckState) running() bool {
	return c.stage != checkOff && c.stage != checkDone
}

func (a App) updateLevel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := &a.check
	k := msg.String()
	if k == "x" && c.running() {
		a.check = levelCheckState{req: c.req}
		return a, nil
	}
	switch c.stage {
	case checkOff:
		if k == "enter" && a.teacher != nil && a.readiness().Ready() {
			next, _ := a.profile.Level.Next()
			a.check = levelCheckState{next: next, from: a.profile.Level, req: c.req}
			return a.askCheckReading()
		}
	case checkReadingFail:
		if k == "enter" {
			return a.askCheckReading()
		}
	case checkReading:
		q := c.reading.Questions[len(c.picked)]
		if n, ok := optionKey(k, len(q.Options)); ok {
			c.picked = append(c.picked, n)
			if len(c.picked) == len(c.reading.Questions) {
				c.result.Reading = c.readingScore()
				c.stage = checkReadingDone
			}
		}
	case checkReadingDone:
		if k == "enter" {
			c.stage = checkWriting
			a.screen = Write
			return a.askTask()
		}
	case checkQuizReady:
		if k == "enter" {
			c.stage = checkQuiz
			a.screen = Quiz
			a.quiz.cursor = quizGrammar
			return a.askGrammar()
		}
	case checkDone:
		if k == "enter" {
			a.check = levelCheckState{req: c.req}
		}
	}
	return a, nil
}

// readingScore is how many reading questions were answered right.
func (c levelCheckState) readingScore() progress.Score {
	s := progress.Score{Total: len(c.reading.Questions)}
	for i, n := range c.picked {
		if n == c.reading.Questions[i].Answer {
			s.Right++
		}
	}
	return s
}

// askCheckReading asks the teacher for the check's reading at the next level.
func (a App) askCheckReading() (App, tea.Cmd) {
	c := &a.check
	c.req++
	c.stage, c.err = checkReadingWait, nil
	req, tc, level := c.req, a.teacher, c.next
	return a, func() tea.Msg {
		r, err := tc.CheckReading(context.Background(), level)
		return checkReadingMsg{req: req, reading: r, err: err}
	}
}

func (a App) gotCheckReading(msg checkReadingMsg) App {
	c := &a.check
	if msg.req != c.req || c.stage != checkReadingWait {
		return a
	}
	if msg.err == nil && len(msg.reading.Questions) == 0 {
		msg.err = errors.New("the reading came back without questions")
	}
	if msg.err != nil {
		c.stage, c.err = checkReadingFail, msg.err
		return a
	}
	c.stage, c.reading, c.picked = checkReading, msg.reading, nil
	return a
}

// checkWritten takes the rubric total of a corrected text: during the
// check's writing it is the writing score, and the quiz comes next.
func (a *App) checkWritten(total int) {
	if a.check.stage != checkWriting {
		return
	}
	a.check.result.Writing = progress.Score{Right: total, Total: teacher.MaxTotal}
	a.check.stage = checkQuizReady
	a.screen = Level
}

// checkQuizzed takes the score of a finished grammar quiz: during the
// check's quiz it ends the check.
func (a *App) checkQuizzed(right, total int) {
	if a.check.stage != checkQuiz {
		return
	}
	a.check.result.Quiz = progress.Score{Right: right, Total: total}
	a.finishCheck()
}

// finishCheck grades the check. Passing raises the level and restarts its
// clock today; that is the only way the app raises a level. A failed save
// keeps the old level. Either way the result is recorded as an activity.
func (a *App) finishCheck() {
	c := &a.check
	c.stage, c.show = checkDone, true
	c.passed = c.result.Passed()
	if !c.passed {
		a.addToToday(domain.Day{Activities: []string{"Level check: not yet"}})
		return
	}
	p := a.profile
	p.Level, p.Started = c.next, a.today()
	if a.saveProfile != nil {
		if err := a.saveProfile(p); err != nil {
			c.err = err
			return
		}
	}
	a.profile = p
	a.addToToday(domain.Day{Activities: []string{"Level check: passed " + c.next.String()}})
}

// showCheckResult brings the learner back to the Level screen once, when
// they leave the last answer of the check's quiz.
func (a *App) showCheckResult() {
	if a.check.stage == checkDone && a.check.show {
		a.check.show = false
		a.screen = Level
	}
}

// levelHints are the keys of the Level screen.
func (a App) levelHints() []string {
	c := a.check
	switch c.stage {
	case checkOff:
		if a.teacher != nil && a.readiness().Ready() {
			return []string{"enter level check", "esc home", "? help"}
		}
	case checkReadingFail:
		return []string{"enter try again", "x stop check", "esc home"}
	case checkReading:
		return []string{"1–4 choose", "x stop check", "esc home"}
	case checkReadingDone:
		return []string{"enter writing", "x stop check", "esc home"}
	case checkQuizReady:
		return []string{"enter grammar quiz", "x stop check", "esc home"}
	case checkDone:
		return []string{"enter back", "esc home", "? help"}
	}
	return []string{"esc home", "? help"}
}

// levelLine is the Home line about the next level, like
// "A1 → A2 · 3 of 6 ready".
func (a App) levelLine() string {
	r := a.st.main
	rd := a.readiness()
	key := r.label.Render("   ") + r.accent.Render("l")
	switch {
	case rd.Next == "":
		return r.value.Render(rd.Level.String()) + r.secondary.Render(" · the top level") + key
	case a.check.running():
		return r.value.Render(fmt.Sprintf("%s → %s", rd.Level, rd.Next)) + r.secondary.Render(" · level check in progress") + key
	case rd.Ready():
		return r.value.Render(rd.Level.String()) + r.label.Render(" · ") +
			r.success.Render("Ready for "+rd.Next.String()) + r.secondary.Render(" · take the level check") + key
	}
	return r.value.Render(fmt.Sprintf("%s → %s", rd.Level, rd.Next)) +
		r.secondary.Render(fmt.Sprintf(" · %d of %d ready", rd.MetCount(), len(rd.Criteria))) + key
}
