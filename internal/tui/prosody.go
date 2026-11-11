package tui

import (
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// Prosody (spec S6) lives in Pairs, like the minimal pairs: `m` on an
// intro switches between minimal pairs, word stress and intonation. A
// word-stress round plays a word and the learner chooses its stressed
// syllable; an intonation round plays a sentence as a question (rising)
// or a statement (falling) and the learner chooses which. The answers
// are built-in data (domain.StressDrills, domain.ToneDrills).
//
// A round saves like a minimal-pairs round: at its last answer, once,
// the activity in today's log and each drill's results in the pair
// stats, keyed by drill ID ("stress:…", "tone:…").

// pairsKind is what the Pairs screen trains.
type pairsKind int

const (
	pairsMinimal pairsKind = iota // minimal pairs: the zero value
	pairsStress                   // word stress
	pairsTone                     // intonation
)

// pairsKinds is how many kinds `m` goes through.
const pairsKinds = 3

// String is the kind as `m` names it.
func (k pairsKind) String() string {
	return [...]string{"minimal pairs", "stress", "intonation"}[k]
}

// stressItems and toneItems are how many words or sentences one round
// plays.
const (
	stressItems = 15
	toneItems   = 12
)

// prosodyState is a word-stress or intonation round. Its phases are the
// ones of a minimal-pairs round.
type prosodyState struct {
	phase     pairsPhase
	count     int // rounds started, mixed into the seed so each one differs
	items     []domain.ProsodyItem
	chosen    []int     // the option chosen for each answered item
	pos       int       // the item on screen
	startedAt time.Time // when the first item played, for the study minutes
	saveErr   error     // why the round's results could not be saved
}

// rights are the answers so far, right or not, in order.
func (p prosodyState) rights() []bool {
	out := make([]bool, len(p.chosen))
	for i, c := range p.chosen {
		out[i] = c == p.items[i].Answer
	}
	return out
}

// right reports whether item i was answered right.
func (p prosodyState) right(i int) bool { return p.chosen[i] == p.items[i].Answer }

// score is how many answers were right.
func (p prosodyState) score() int {
	n := 0
	for _, ok := range p.rights() {
		if ok {
			n++
		}
	}
	return n
}

// prosodyDrills are the drills of the kind on screen.
func (a App) prosodyDrills() []domain.Drill {
	if a.pairs.kind == pairsTone {
		return domain.ToneDrills()
	}
	return domain.StressDrills()
}

// prosodyItems is how many items a round of the kind on screen plays.
func (a App) prosodyItems() int {
	if a.pairs.kind == pairsTone {
		return toneItems
	}
	return stressItems
}

// prosodyOrder is the drills of the kind on screen, weakest first, as
// the next round puts them.
func (a App) prosodyOrder() []domain.Drill {
	return domain.WeakestDrills(a.prosodyDrills(), a.pairStats)
}

// nextPairsKind switches the Pairs screen to the next kind, on an intro.
func (a App) nextPairsKind() App {
	a.pairs.kind = (a.pairs.kind + 1) % pairsKinds
	a.prosody = prosodyState{count: a.prosody.count}
	return a
}

// startProsody plays the first item of a new round.
func (a App) startProsody() (App, tea.Cmd) {
	if a.speaker == nil {
		return a, nil
	}
	seed := uint64(a.now().UnixNano())
	rng := rand.New(rand.NewPCG(seed, uint64(a.prosody.count+1)))
	a.prosody = prosodyState{
		phase: pairsAsking, count: a.prosody.count + 1, startedAt: a.now(),
		items: domain.ProsodyRound(a.prosodyDrills(), a.pairStats, a.prosodyItems(), rng.IntN),
	}
	return a, a.playProsody(false)
}

// playProsody says the item on screen. Word stress takes turns in the
// installed voices like the minimal pairs; intonation keeps the
// configured voice, whose rise and fall were checked.
func (a App) playProsody(slow bool) tea.Cmd {
	p := a.prosody
	voice := ""
	if a.pairs.kind == pairsStress {
		voice = a.pairVoice(p.pos)
	}
	return a.say([]string{p.items[p.pos].Say}, speech.Options{Slow: slow, Voice: voice})
}

// saveProsody adds the round to the stored totals and to today's log,
// like savePairs.
func (a *App) saveProsody() {
	p := &a.prosody
	results := domain.TallyProsody(p.items, p.rights())
	if a.recordPairs != nil {
		p.saveErr = a.recordPairs(results)
	}
	if p.saveErr == nil {
		a.addPairStats(results)
	}
	minutes := min(int(a.now().Sub(p.startedAt)/time.Minute), maxPairsMinutes)
	name := "Stress"
	if a.pairs.kind == pairsTone {
		name = "Intonation"
	}
	activity := fmt.Sprintf("%s: %d/%d", name, p.score(), len(p.items))
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
}

func (a App) updateProsody(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &a.prosody
	k := msg.String()
	switch p.phase {
	case pairsIntro:
		switch k {
		case "enter":
			return a.startProsody()
		case "m":
			return a.nextPairsKind(), nil
		}
	case pairsAsking:
		if n, ok := choiceKey(k, len(p.items[p.pos].Choices)); ok {
			p.chosen = append(p.chosen, n)
			p.phase = pairsAnswered
			if len(p.chosen) == len(p.items) {
				a.saveProsody()
			}
			return a, nil
		}
		if k == "p" || k == "s" {
			return a, a.playProsody(k == "s")
		}
	case pairsAnswered:
		switch k {
		case "p", "s":
			return a, a.playProsody(k == "s")
		case "enter":
			if p.pos+1 < len(p.items) {
				p.pos++
				p.phase = pairsAsking
				return a, a.playProsody(false)
			}
			p.phase = pairsResult
		}
	case pairsResult:
		if k == "n" {
			a.prosody = prosodyState{count: p.count}
		}
	}
	return a, nil
}

// choiceKey is the option of key k, "1" to "n", from 0.
func choiceKey(k string, n int) (int, bool) {
	if len(k) != 1 || k[0] < '1' || int(k[0]-'1') >= n {
		return 0, false
	}
	return int(k[0] - '1'), true
}

// drillByID is the built-in stress or intonation drill with id; the zero
// Drill when there is none.
func drillByID(id string) domain.Drill {
	for _, d := range append(domain.StressDrills(), domain.ToneDrills()...) {
		if d.ID == id {
			return d
		}
	}
	return domain.Drill{}
}
