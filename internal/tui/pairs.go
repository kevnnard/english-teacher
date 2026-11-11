package tui

import (
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// Minimal pairs (spec S5): the Pairs screen plays one word of a pair such
// as ship / sheep, and the learner chooses which one it was. The results
// of each contrast are kept, so the next round starts with the weakest.
//
// A round saves at its last answer, once: the activity in today's log and
// the results of every contrast. A round left midway saves nothing, so
// the stored totals only ever hold whole rounds.

// pairsItems is how many words one round plays.
const pairsItems = 20

// maxPairsMinutes caps the study time one round can record, like
// maxQuizMinutes: a round left open does not fill the daily goal.
const maxPairsMinutes = maxQuizMinutes

// pairsPhase is where the learner is in a round.
type pairsPhase int

const (
	pairsIntro    pairsPhase = iota // what the round is; enter starts
	pairsAsking                     // the word played; the learner chooses 1 or 2
	pairsAnswered                   // the feedback of the word on screen
	pairsResult                     // every word is answered
)

// pairsState is the Pairs screen: one round, its answers and its result.
type pairsState struct {
	phase     pairsPhase
	count     int // rounds started, mixed into the seed so each one differs
	items     []domain.PairItem
	chosen    []int     // the word chosen for each answered item: 0 or 1
	pos       int       // the item on screen
	startedAt time.Time // when the first word played, for the study minutes
	saveErr   error     // why the round's results could not be saved
}

// right reports whether item i was answered right.
func (p pairsState) right(i int) bool { return p.chosen[i] == p.items[i].Played }

// rights are the answers so far, right or not, in order.
func (p pairsState) rights() []bool {
	out := make([]bool, len(p.chosen))
	for i := range p.chosen {
		out[i] = p.right(i)
	}
	return out
}

// score is how many answers were right.
func (p pairsState) score() int {
	n := 0
	for _, ok := range p.rights() {
		if ok {
			n++
		}
	}
	return n
}

// pairsOrder is the contrasts weakest first, as the next round puts
// them.
func (a App) pairsOrder() []domain.Contrast {
	return domain.WeakestFirst(domain.Contrasts(), a.pairStats)
}

// startPairs plays the first word of a new round.
func (a App) startPairs() (App, tea.Cmd) {
	if a.speaker == nil {
		return a, nil
	}
	seed := uint64(a.now().UnixNano())
	rng := rand.New(rand.NewPCG(seed, uint64(a.pairs.count+1)))
	a.pairs = pairsState{
		phase: pairsAsking, count: a.pairs.count + 1, startedAt: a.now(),
		items: domain.PairRound(domain.Contrasts(), a.pairStats, pairsItems, rng.IntN),
	}
	return a, a.playPair(-1, false)
}

// pairVoice is the voice of item i: the installed voices take turns when
// there are two or more, else the configured one ("").
func (a App) pairVoice(i int) string {
	if len(a.voices) < 2 {
		return ""
	}
	return a.voices[i%len(a.voices)]
}

// playPair says a word of the item on screen in the item's voice: the
// word played when which is -1, else word which (0 or 1) to compare.
func (a App) playPair(which int, slow bool) tea.Cmd {
	p := a.pairs
	it := p.items[p.pos]
	word := it.Word()
	if which >= 0 {
		word = it.Pair[which]
	}
	return a.say([]string{word}, speech.Options{Slow: slow, Voice: a.pairVoice(p.pos)})
}

// answerPair records the word chosen for the item on screen. The last
// answer saves the round, once.
func (a *App) answerPair(which int) {
	p := &a.pairs
	p.chosen = append(p.chosen, which)
	p.phase = pairsAnswered
	if len(p.chosen) < len(p.items) {
		return
	}
	a.savePairs()
}

// savePairs adds the round to the stored totals and to today's log. A
// failed stats save is shown on the result; the round still counts.
func (a *App) savePairs() {
	p := &a.pairs
	results := domain.TallyPairs(p.items, p.rights())
	if a.recordPairs != nil {
		p.saveErr = a.recordPairs(results)
	}
	if p.saveErr == nil {
		if a.pairStats == nil {
			a.pairStats = map[string]domain.PairStat{}
		}
		for _, r := range results {
			st := a.pairStats[r.Contrast]
			st.Contrast = r.Contrast
			st.Attempts += r.Attempts
			st.Correct += r.Correct
			a.pairStats[r.Contrast] = st
		}
	}
	minutes := min(int(a.now().Sub(p.startedAt)/time.Minute), maxPairsMinutes)
	activity := fmt.Sprintf("Pairs: %d/%d", p.score(), len(p.items))
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
}

func (a App) updatePairs(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &a.pairs
	k := msg.String()
	switch p.phase {
	case pairsIntro:
		if k == "enter" {
			return a.startPairs()
		}
	case pairsAsking:
		switch k {
		case "1", "2":
			a.answerPair(int(k[0] - '1'))
		case "p", "s":
			return a, a.playPair(-1, k == "s")
		}
	case pairsAnswered:
		switch k {
		case "1", "2":
			return a, a.playPair(int(k[0]-'1'), false) // hear each word to compare
		case "p", "s":
			return a, a.playPair(-1, k == "s")
		case "enter":
			if p.pos+1 < len(p.items) {
				p.pos++
				p.phase = pairsAsking
				return a, a.playPair(-1, false)
			}
			p.phase = pairsResult
		}
	case pairsResult:
		if k == "n" {
			a.pairs = pairsState{count: p.count}
		}
	}
	return a, nil
}
