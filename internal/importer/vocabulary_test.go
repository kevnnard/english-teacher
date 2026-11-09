package importer

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestParseVocabularyBankFixture(t *testing.T) {
	f, err := os.Open("testdata/vocabulary-bank.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	cards, err := ParseVocabularyBank(f)
	if err != nil {
		t.Fatalf("ParseVocabularyBank: %v", err)
	}

	reviewed := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	want := []domain.Card{
		{Word: "hello", Pronunciation: "/həˈloʊ/", Meaning: "Hola", Example: "Hello, how are you?",
			TimesCorrect: 1, LastReviewed: &reviewed, EaseFactor: domain.DefaultEaseFactor},
		{Word: "hi", Pronunciation: "/haɪ/", Meaning: "Hola (informal)", Example: "Hi, nice to meet you!",
			TimesCorrect: 1, LastReviewed: &reviewed, EaseFactor: domain.DefaultEaseFactor},
		{Word: "Argentina", Pronunciation: "/ˌɑːrdʒənˈtiːnə/", Meaning: "País: Argentina", Example: "I am from Argentina.",
			EaseFactor: domain.DefaultEaseFactor},
	}
	if len(cards) != len(want) {
		t.Fatalf("got %d cards, want %d", len(cards), len(want))
	}
	for i := range want {
		assertCard(t, cards[i], want[i])
	}
}

func TestParseVocabularyBankValues(t *testing.T) {
	const header = "| word | pronunciation | meaning | example | status | times_failed | times_correct | last_reviewed |\n|---|---|---|---|---|---|---|---|\n"
	tests := []struct {
		name         string
		row          string
		wantFailed   int
		wantCorrect  int
		wantReviewed string // "" means nil
	}{
		{name: "em dash means never reviewed", row: "| a | /a/ | A | A. | new | 0 | 0 | — |", wantReviewed: ""},
		{name: "hyphen means never reviewed", row: "| a | /a/ | A | A. | new | 0 | 0 | - |", wantReviewed: ""},
		{name: "empty means never reviewed", row: "| a | /a/ | A | A. | new | 0 | 0 |  |", wantReviewed: ""},
		{name: "date and counters", row: "| a | /a/ | A | A. | learning | 2 | 7 | 2026-10-31 |", wantFailed: 2, wantCorrect: 7, wantReviewed: "2026-10-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards, err := ParseVocabularyBank(strings.NewReader(header + tt.row + "\n"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cards) != 1 {
				t.Fatalf("got %d cards, want 1", len(cards))
			}
			c := cards[0]
			if c.TimesFailed != tt.wantFailed || c.TimesCorrect != tt.wantCorrect {
				t.Errorf("failed=%d correct=%d, want failed=%d correct=%d", c.TimesFailed, c.TimesCorrect, tt.wantFailed, tt.wantCorrect)
			}
			got := ""
			if c.LastReviewed != nil {
				got = c.LastReviewed.Format(DateLayout)
			}
			if got != tt.wantReviewed {
				t.Errorf("LastReviewed = %q, want %q", got, tt.wantReviewed)
			}
		})
	}
}

func TestParseVocabularyBankColumnOrderAndStatusOptional(t *testing.T) {
	in := "| meaning | word | example | pronunciation | last_reviewed | times_correct | times_failed |\n" +
		"|---|---|---|---|---|---|---|\n" +
		"| Hola | hello | Hello! | /h/ | 2026-08-14 | 3 | 1 |\n"

	cards, err := ParseVocabularyBank(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 1 || cards[0].Word != "hello" || cards[0].Meaning != "Hola" || cards[0].TimesCorrect != 3 {
		t.Errorf("got %+v, want hello/Hola/3", cards)
	}
}

func TestParseVocabularyBankErrors(t *testing.T) {
	const header = "| word | pronunciation | meaning | example | status | times_failed | times_correct | last_reviewed |\n|---|---|---|---|---|---|---|---|\n"
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{name: "no table", in: "# Vocabulary Bank\n\nnothing here\n", wantErr: "no table"},
		{name: "missing meaning column", in: "| word | pronunciation | example | times_failed | times_correct | last_reviewed |\n|---|---|---|---|---|---|\n", wantErr: `missing column "meaning"`},
		{name: "missing word column", in: "| pronunciation | meaning | example | times_failed | times_correct | last_reviewed |\n", wantErr: `missing column "word"`},
		{name: "bad times_failed", in: header + "| a | /a/ | A | A. | new | x | 0 | — |\n", wantErr: "line 3"},
		{name: "bad date", in: header + "| a | /a/ | A | A. | new | 0 | 0 | 14/08/2026 |\n", wantErr: "last_reviewed"},
		{name: "wrong cell count", in: header + "| a | /a/ | A |\n", wantErr: "expected 8 cells"},
		{name: "empty word", in: header + "|  | /a/ | A | A. | new | 0 | 0 | — |\n", wantErr: "empty word"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseVocabularyBank(strings.NewReader(tt.in))
			if err == nil {
				t.Fatalf("error = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func assertCard(t *testing.T, got, want domain.Card) {
	t.Helper()
	if got.Word != want.Word || got.Pronunciation != want.Pronunciation ||
		got.Meaning != want.Meaning || got.Example != want.Example ||
		got.TimesCorrect != want.TimesCorrect || got.TimesFailed != want.TimesFailed ||
		got.EaseFactor != want.EaseFactor {
		t.Errorf("card mismatch:\n got  %+v\n want %+v", got, want)
	}
	switch {
	case want.LastReviewed == nil && got.LastReviewed != nil:
		t.Errorf("%s: LastReviewed = %v, want nil", want.Word, got.LastReviewed)
	case want.LastReviewed != nil && (got.LastReviewed == nil || !got.LastReviewed.Equal(*want.LastReviewed)):
		t.Errorf("%s: LastReviewed = %v, want %v", want.Word, got.LastReviewed, want.LastReviewed)
	}
}
