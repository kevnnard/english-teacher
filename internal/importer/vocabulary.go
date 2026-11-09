// Package importer reads learner data exported by the english-master project.
package importer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// DateLayout is the date format used in the last_reviewed column.
const DateLayout = "2006-01-02"

// requiredColumns must all appear in the table header. Other columns, such as
// status, are ignored.
var requiredColumns = []string{
	"word", "pronunciation", "meaning", "example",
	"times_failed", "times_correct", "last_reviewed",
}

// ParseVocabularyBank parses the first markdown table in r into cards.
//
// The header row names the columns, so their order does not matter. In
// last_reviewed, "—", "-" or an empty cell means the card was never reviewed.
// Every card starts with a fresh SM-2 schedule and is due immediately.
func ParseVocabularyBank(r io.Reader) ([]domain.Card, error) {
	var (
		cards  []domain.Card
		col    map[string]int // column name -> cell index; nil until the header is read
		width  int
		lineNo int
	)

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "|") {
			if col != nil {
				break // the table has ended
			}
			continue // text before the table
		}

		cells := splitRow(line)
		if col == nil {
			var err error
			if col, err = parseHeader(cells); err != nil {
				return nil, err
			}
			width = len(cells)
			continue
		}
		if isSeparator(cells) {
			continue
		}
		if len(cells) != width {
			return nil, fmt.Errorf("line %d: expected %d cells, got %d", lineNo, width, len(cells))
		}

		card, err := parseRow(cells, col)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		cards = append(cards, card)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if col == nil {
		return nil, errors.New("no table found")
	}
	return cards, nil
}

// splitRow turns "| a | b |" into ["a", "b"].
func splitRow(line string) []string {
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i, c := range cells {
		cells[i] = strings.TrimSpace(c)
	}
	return cells
}

func parseHeader(cells []string) (map[string]int, error) {
	col := make(map[string]int, len(cells))
	for i, name := range cells {
		col[strings.ToLower(name)] = i
	}
	for _, name := range requiredColumns {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing column %q", name)
		}
	}
	return col, nil
}

// isSeparator reports whether cells is the "|---|---|" row under the header.
func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return true
}

func parseRow(cells []string, col map[string]int) (domain.Card, error) {
	get := func(name string) string { return cells[col[name]] }

	word := get("word")
	if word == "" {
		return domain.Card{}, errors.New("empty word")
	}

	card := domain.NewCard(word)
	card.Pronunciation = get("pronunciation")
	card.Meaning = get("meaning")
	card.Example = get("example")

	var err error
	if card.TimesFailed, err = parseCount(get("times_failed")); err != nil {
		return domain.Card{}, fmt.Errorf("times_failed: %w", err)
	}
	if card.TimesCorrect, err = parseCount(get("times_correct")); err != nil {
		return domain.Card{}, fmt.Errorf("times_correct: %w", err)
	}
	if card.LastReviewed, err = parseDate(get("last_reviewed")); err != nil {
		return domain.Card{}, fmt.Errorf("last_reviewed: %w", err)
	}
	return card, nil
}

func parseCount(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("%d is negative", n)
	}
	return n, nil
}

// parseDate returns nil for "never reviewed" markers.
func parseDate(s string) (*time.Time, error) {
	switch s {
	case "", "—", "-":
		return nil, nil
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return nil, fmt.Errorf("%q is not a %s date", s, DateLayout)
	}
	return &t, nil
}
