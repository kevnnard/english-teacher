package importer

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var aug14 = domain.Date{Year: 2026, Month: time.August, Day: 14}

func TestParseLevelFile(t *testing.T) {
	in := "# English Level Progress\n**Current Level:** A1 (Beginner)\n**Started:** 2026-08-14\n**Last Updated:** 2026-08-14\n"

	level, started, err := ParseLevelFile(strings.NewReader(in))

	if err != nil || level != domain.A1 || started != aug14 {
		t.Errorf("ParseLevelFile = %v %v %v, want A1 2026-08-14", level, started, err)
	}
}

func TestParseLevelFileErrors(t *testing.T) {
	for name, in := range map[string]string{
		"no level line": "# English Level Progress\n**Started:** 2026-08-14\n",
		"bad level":     "**Current Level:** Z9 (Unknown)\n",
		"bad start":     "**Current Level:** A1\n**Started:** August 14\n",
	} {
		if _, _, err := ParseLevelFile(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseLevelFileWithoutStart(t *testing.T) {
	_, started, err := ParseLevelFile(strings.NewReader("**Current Level:** b2\n"))
	if err != nil || started != (domain.Date{}) {
		t.Errorf("started = %v, %v; want zero", started, err)
	}
}

func TestParseStreakHistory(t *testing.T) {
	in := "# Daily Streak\n**Current Streak:** 1 day\n\n**Streak history:**\n" +
		"- 2026-08-14 — Lesson 1 (Greetings) + Homework review + Lesson 2 (Expanding Introductions) ✓\n" +
		"- 2026-08-15 - Quiz ✓\n"

	days, err := ParseStreakHistory(strings.NewReader(in))

	want := []domain.Day{
		{Date: aug14, Activities: []string{"Lesson 1 (Greetings)", "Homework review", "Lesson 2 (Expanding Introductions)"}},
		{Date: aug14.AddDays(1), Activities: []string{"Quiz"}},
	}
	if err != nil || !reflect.DeepEqual(days, want) {
		t.Errorf("ParseStreakHistory = %+v, %v\nwant %+v", days, err, want)
	}
}

func TestParseStreakHistoryRejectsABadDate(t *testing.T) {
	_, err := ParseStreakHistory(strings.NewReader("**Streak history:**\n- 2026-13-40 — Lesson ✓\n"))
	if err == nil {
		t.Error("want an error for an impossible date")
	}
}

func TestParseDailyLog(t *testing.T) {
	in := "# Daily Learning Log\n\n## 2026-08-14 (Session 2)\n\n- Lesson 1 reviewed\n\n" +
		"## 2026-08-14 (Session 1)\n\n- Started at A1 level\n\n## 2026-08-16\n- Quiz\n"

	days, err := ParseDailyLog(strings.NewReader(in))

	want := []domain.Day{
		{Date: aug14, Activities: []string{"Session 2", "Session 1"}},
		{Date: aug14.AddDays(2), Activities: []string{"Study session"}},
	}
	if err != nil || !reflect.DeepEqual(days, want) {
		t.Errorf("ParseDailyLog = %+v, %v\nwant %+v", days, err, want)
	}
}

func TestParseDailyLogRejectsABadHeading(t *testing.T) {
	if _, err := ParseDailyLog(strings.NewReader("## yesterday (Session 1)\n")); err == nil {
		t.Error("want an error for a heading that is not a date")
	}
}
