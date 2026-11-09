package importer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// ParseLevelFile reads level.md: the CEFR level from the
// "**Current Level:** A1 (Beginner)" line and, when present, the start date
// from "**Started:** 2026-08-14". A missing start date comes back as zero.
func ParseLevelFile(r io.Reader) (domain.Level, domain.Date, error) {
	var (
		level   domain.Level
		started domain.Date
	)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := boldField(line, "Current Level"); ok {
			code, _, _ := strings.Cut(v, " ") // "A1 (Beginner)" -> "A1"
			l, err := domain.ParseLevel(code)
			if err != nil {
				return "", started, err
			}
			level = l
		}
		if v, ok := boldField(line, "Started"); ok {
			d, err := domain.ParseDate(v)
			if err != nil {
				return "", started, fmt.Errorf("started: %w", err)
			}
			started = d
		}
	}
	if err := sc.Err(); err != nil {
		return "", started, err
	}
	if level == "" {
		return "", started, errors.New(`no "**Current Level:**" line`)
	}
	return level, started, nil
}

// ParseStreakHistory reads the "- 2026-08-14 — Lesson 1 + Homework review ✓"
// lines of streak.md. Each line is one study day; its activities are split
// on "+" and the trailing ✓ is dropped. The streak counts in the file are
// ignored: the app computes the streak from the days themselves.
func ParseStreakHistory(r io.Reader) ([]domain.Day, error) {
	var days []domain.Day
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		rest, ok := strings.CutPrefix(line, "- ")
		if !ok || len(rest) < 10 {
			continue
		}
		date, err := domain.ParseDate(rest[:10])
		if err != nil {
			if looksLikeDate(rest[:10]) {
				return nil, err
			}
			continue // a bullet that is not a history line
		}
		text := strings.TrimSpace(rest[10:])
		text = strings.TrimSpace(strings.TrimLeft(text, "—-–"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "✓"))

		day := domain.Day{Date: date}
		for _, a := range strings.Split(text, "+") {
			if a = strings.TrimSpace(a); a != "" {
				day.Activities = append(day.Activities, a)
			}
		}
		days = append(days, day)
	}
	return days, sc.Err()
}

// ParseDailyLog reads the "## 2026-08-14 (Session 2)" headings of
// daily-log.md. Every heading is one activity on its day, named after the
// text in parentheses, or "Study session" when there is none. Sessions on
// the same day are merged into one day, in the order they appear.
func ParseDailyLog(r io.Reader) ([]domain.Day, error) {
	var days []domain.Day
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "## ")
		if !ok {
			continue
		}
		dateText, label, _ := strings.Cut(strings.TrimSpace(rest), " ")
		date, err := domain.ParseDate(dateText)
		if err != nil {
			return nil, fmt.Errorf("heading %q: %w", rest, err)
		}
		label = strings.Trim(strings.TrimSpace(label), "()")
		if label == "" {
			label = "Study session"
		}

		if n := len(days); n > 0 && days[n-1].Date == date {
			days[n-1].Activities = append(days[n-1].Activities, label)
			continue
		}
		days = append(days, domain.Day{Date: date, Activities: []string{label}})
	}
	return days, sc.Err()
}

// boldField returns the value of a "**Name:** value" line.
func boldField(line, name string) (string, bool) {
	v, ok := strings.CutPrefix(line, "**"+name+":**")
	return strings.TrimSpace(v), ok
}

// looksLikeDate reports whether s has the shape NNNN-NN-NN.
func looksLikeDate(s string) bool {
	for i, r := range s {
		if (i == 4 || i == 7) != (r == '-') || (r != '-' && (r < '0' || r > '9')) {
			return false
		}
	}
	return len(s) == 10
}
