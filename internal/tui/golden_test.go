package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/")

// TestGolden keeps a plain-text snapshot of each key screen at the three
// reference widths, so layout changes show up in review as a text diff.
// Run `go test ./internal/tui -run Golden -update` after an intended change.
func TestGolden(t *testing.T) {
	cases := []struct {
		name  string
		start Screen
		keys  []string
	}{
		{name: "home", start: Home},
		{name: "review", start: Review, keys: []string{"space"}},
	}
	for _, c := range cases {
		for _, width := range []int{80, 120, 160} {
			name := fmt.Sprintf("%s-%d", c.name, width)
			t.Run(name, func(t *testing.T) {
				a := newApp(t, deck(), &recorder{}, c.start, width, 24)
				a, _ = press(t, a, c.keys...)
				got := trimRight(screen(a))

				path := filepath.Join("testdata", name+".golden")
				if *update {
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update to create it)", err)
				}
				if got != string(want) {
					t.Errorf("view differs from %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
				}
			})
		}
	}
}

// trimRight drops trailing spaces so golden files stay readable.
func trimRight(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
