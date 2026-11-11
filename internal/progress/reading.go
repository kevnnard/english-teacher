package progress

import (
	"strings"
	"unicode"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// CountWords counts the words of text: the pieces between spaces that
// have at least one letter or digit. "don't" and "well-known" are one
// word each, and a dash or other punctuation on its own is none.
func CountWords(text string) int {
	n := 0
	for _, f := range strings.Fields(text) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			n++
		}
	}
	return n
}

// WeekStart is the Monday of the week of d. Weeks run Monday to Sunday,
// so on a Sunday it is six days before.
func WeekStart(d domain.Date) domain.Date {
	sinceMonday := (int(d.Weekday()) + 6) % 7 // time.Sunday is 0
	return d.AddDays(-sinceMonday)
}

// WordsReadThisWeek sums the words read from the Monday of today's week
// up to today, both included. It resets every Monday, so "this week" means
// the calendar week, not the last seven days; days after today are ignored.
func WordsReadThisWeek(days []domain.Day, today domain.Date) int {
	monday := WeekStart(today)
	total := 0
	for _, d := range days {
		if !d.Date.Before(monday) && !today.Before(d.Date) {
			total += d.WordsRead
		}
	}
	return total
}
