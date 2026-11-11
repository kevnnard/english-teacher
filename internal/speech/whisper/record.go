package whisper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// DefaultStopTimeout is how long a recorder may take to finish its file
// after the interrupt before it is killed.
const DefaultStopTimeout = 3 * time.Second

// wavHeader is the size of a WAV header: a file this small holds no
// sound.
const wavHeader = 44

// Process is a running program, like a started *exec.Cmd.
type Process interface {
	Interrupt() error // ask it to finish, like ctrl+c (SIGINT)
	Kill() error      // end it now
	Wait() error      // wait until it has ended
}

// Launcher starts a program, like execLaunch.
type Launcher func(ctx context.Context, name string, args []string) (Process, error)

// Recorder records the microphone with a recorder program, 16 kHz mono
// as whisper wants it. It implements speech.Recorder.
type Recorder struct {
	Program     string        // the recorder program: pw-record, arecord or sox
	Launch      Launcher      // execLaunch when nil; tests use a fake
	TempDir     string        // where the WAV is written; os.TempDir() when empty
	StopTimeout time.Duration // DefaultStopTimeout when zero
}

// recorderArgs are the arguments that make program record 16 kHz, mono,
// 16-bit audio into wav until it is interrupted.
func recorderArgs(program, wav string) []string {
	switch filepath.Base(program) {
	case "arecord":
		return []string{"-q", "-f", "S16_LE", "-r", "16000", "-c", "1", wav}
	case "sox", "rec":
		return []string{"-q", "-d", "-r", "16000", "-c", "1", "-b", "16", wav}
	}
	return []string{"--rate", "16000", "--channels", "1", "--format", "s16", wav}
}

// Start begins recording into a new temporary WAV file.
func (r *Recorder) Start(ctx context.Context) (speech.Recording, error) {
	launch, timeout := r.Launch, r.StopTimeout
	if launch == nil {
		launch = execLaunch
	}
	if timeout == 0 {
		timeout = DefaultStopTimeout
	}
	f, err := os.CreateTemp(r.TempDir, "teacher-rec-*.wav")
	if err != nil {
		return nil, err
	}
	wav := f.Name()
	f.Close()

	proc, err := launch(ctx, r.Program, recorderArgs(r.Program, wav))
	if err != nil {
		os.Remove(wav)
		return nil, explain(err, RecorderHint)
	}
	rec := &recording{proc: proc, wav: wav, timeout: timeout, done: make(chan struct{})}
	go func() {
		rec.waitErr = proc.Wait()
		close(rec.done)
	}()
	return rec, nil
}

// recording is one recorder program at work. It implements
// speech.Recording.
type recording struct {
	proc    Process
	wav     string
	timeout time.Duration
	done    chan struct{} // closed once the program has ended
	waitErr error         // why it ended; read only after done

	once sync.Once // the recording ends once, by Stop or Cancel
}

// ended reports whether the program has already ended by itself.
func (r *recording) ended() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// Stop interrupts the recorder, which then finishes the WAV file, and
// returns the file. A recorder that does not end within the timeout is
// killed. A recording with no sound is an error, and its file is gone.
func (r *recording) Stop() (string, error) {
	err := errors.New("the recording has already ended")
	r.once.Do(func() {
		if r.ended() {
			os.Remove(r.wav)
			err = fmt.Errorf("the microphone did not start (is one plugged in?): %v", r.waitErr)
			return
		}
		r.proc.Interrupt()
		select {
		case <-r.done:
		case <-time.After(r.timeout):
			r.proc.Kill()
			<-r.done
		}
		if info, statErr := os.Stat(r.wav); statErr != nil || info.Size() <= wavHeader {
			os.Remove(r.wav)
			err = errors.New("nothing was recorded: check the microphone")
			return
		}
		err = nil
	})
	if err != nil {
		return "", err
	}
	return r.wav, nil
}

// Cancel kills the recorder and removes its file.
func (r *recording) Cancel() {
	r.once.Do(func() {
		if !r.ended() {
			r.proc.Kill()
		}
		<-r.done
		os.Remove(r.wav)
	})
}

// execProcess is a started *exec.Cmd.
type execProcess struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (p execProcess) Interrupt() error { return p.cmd.Process.Signal(os.Interrupt) }

func (p execProcess) Kill() error { return p.cmd.Process.Kill() }

// Wait waits for the program; a failure carries the last line of stderr.
func (p execProcess) Wait() error {
	if err := p.cmd.Wait(); err != nil {
		return fmt.Errorf("%s: %v: %s", filepath.Base(p.cmd.Path), err, lastLine(p.stderr.String()))
	}
	return nil
}

// execLaunch starts name with no input: the terminal belongs to the app.
func execLaunch(ctx context.Context, name string, args []string) (Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return execProcess{cmd: cmd, stderr: &stderr}, nil
}
