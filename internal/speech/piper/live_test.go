package piper

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// TestLiveSynthesis runs the real piper with the installed voice and
// checks it wrote a WAV. It never plays the file. It only runs with
// TEACHER_LIVE=1, so the normal test suite needs no audio setup.
func TestLiveSynthesis(t *testing.T) {
	bin, voice := livePiper(t)

	wav := filepath.Join(t.TempDir(), "out.wav")
	if err := execRun(context.Background(), bin, []string{"-m", voice, "-f", wav}, "I am thirty years old."); err != nil {
		t.Fatal(err)
	}
	wavData, err := os.ReadFile(wav)
	if err != nil || len(wavData) < 1000 || string(wavData[:4]) != "RIFF" {
		t.Errorf("no WAV written: %d bytes, %v", len(wavData), err)
	}
}

// livePiper is the real piper and the installed default voice; the test
// is skipped unless TEACHER_LIVE=1.
func livePiper(t *testing.T) (bin, voice string) {
	t.Helper()
	if os.Getenv("TEACHER_LIVE") != "1" {
		t.Skip("set TEACHER_LIVE=1 to run the real piper")
	}
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	setup := Setup{VoicesDir: filepath.Join(data, "english-teacher", "voices"), Home: home}
	bin, err := setup.FindPiper()
	if err != nil {
		t.Fatal(err)
	}
	voice, err = setup.FindVoice("")
	if err != nil {
		t.Fatal(err)
	}
	return bin, voice
}

// TestLiveIntonationRisesAndFalls checks the claim of the intonation
// drill with the real piper and voice, at both speeds: said with "." a
// sentence falls (its end well below its middle); said with "?" it does
// not fall and ends at least half as high again as the statement, about
// an octave in practice. Piper adds random noise to each synthesis, so
// the numbers move from run to run. It never plays the audio.
func TestLiveIntonationRisesAndFalls(t *testing.T) {
	bin, voice := livePiper(t)
	drills := domain.ToneDrills()
	for i, q := range drills[0].Items {
		s := drills[1].Items[i]
		for _, slow := range []bool{false, true} {
			qMid, qEnd := liveContour(t, bin, voice, q.Say, slow)
			sMid, sEnd := liveContour(t, bin, voice, s.Say, slow)
			t.Logf("%-24q slow=%-5v question %3.0f→%3.0f Hz, statement %3.0f→%3.0f Hz", q.Shown, slow, qMid, qEnd, sMid, sEnd)
			if !(sEnd < 0.7*sMid && qEnd >= 0.9*qMid && qEnd > 1.5*sEnd) {
				t.Errorf("%q (slow %v): question %.0f→%.0f Hz, statement %.0f→%.0f Hz; want a rise and a fall", q.Shown, slow, qMid, qEnd, sMid, sEnd)
			}
		}
	}
}

// liveContour synthesizes text and returns the mean pitch of the middle
// half of its voiced frames and of its last 10 voiced frames, in Hz.
func liveContour(t *testing.T, bin, voice, text string, slow bool) (mid, end float64) {
	t.Helper()
	wav := filepath.Join(t.TempDir(), "out.wav")
	args := []string{"-m", voice, "-f", wav}
	if slow {
		args = append(args, "--length-scale", SlowLengthScale)
	}
	if err := execRun(context.Background(), bin, args, text); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}
	rate, samples := wavSamples(t, b)
	f0 := pitchFrames(rate, samples)
	if len(f0) < 20 {
		t.Fatalf("%q: %d voiced frames, too few to judge", text, len(f0))
	}
	q := len(f0) / 4
	return mean(f0[q : 3*q]), mean(f0[len(f0)-10:])
}

// wavSamples reads a 16-bit mono PCM WAV, as piper writes it.
func wavSamples(t *testing.T, b []byte) (rate int, x []float64) {
	t.Helper()
	if len(b) < 44 || string(b[:4]) != "RIFF" {
		t.Fatal("not a WAV")
	}
	rate = int(binary.LittleEndian.Uint32(b[24:28]))
	for i := 12; i+8 <= len(b); {
		id, n := string(b[i:i+4]), int(binary.LittleEndian.Uint32(b[i+4:i+8]))
		if id == "data" {
			data := b[i+8 : min(i+8+n, len(b))]
			x = make([]float64, len(data)/2)
			for k := range x {
				x[k] = float64(int16(binary.LittleEndian.Uint16(data[2*k:]))) / 32768
			}
			return rate, x
		}
		i += 8 + n
	}
	t.Fatal("no data chunk in the WAV")
	return 0, nil
}

// pitchFrames is the pitch of each voiced 40 ms frame, every 10 ms, by
// normalized autocorrelation between 70 and 400 Hz. Quiet frames and
// frames without a clear period are left out.
func pitchFrames(rate int, x []float64) []float64 {
	win, hop := rate*40/1000, rate*10/1000
	var out []float64
	for s := 0; s+win < len(x); s += hop {
		fr := x[s : s+win]
		energy := 0.0
		for _, v := range fr {
			energy += v * v
		}
		if energy/float64(win) < 1e-4 {
			continue
		}
		best, lag := 0.0, 0
		for l := rate / 400; l <= rate/70; l++ {
			c, n1, n2 := 0.0, 0.0, 0.0
			for j := 0; j+l < win; j++ {
				c += fr[j] * fr[j+l]
				n1 += fr[j] * fr[j]
				n2 += fr[j+l] * fr[j+l]
			}
			if r := c / math.Sqrt(n1*n2+1e-12); r > best {
				best, lag = r, l
			}
		}
		if best >= 0.6 {
			out = append(out, float64(rate)/float64(lag))
		}
	}
	return out
}

func mean(a []float64) float64 {
	t := 0.0
	for _, v := range a {
		t += v
	}
	return t / float64(len(a))
}
