package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var (
	aug14 = domain.Date{Year: 2026, Month: time.August, Day: 14}
	nov9  = domain.Date{Year: 2026, Month: time.November, Day: 9}
)

func TestDaysStartEmpty(t *testing.T) {
	days, err := openTemp(t).Days(context.Background())
	if err != nil || len(days) != 0 {
		t.Fatalf("Days = %v, %v; want none", days, err)
	}
}

func TestRecordStudyAddsToTheDay(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: 1, Correct: 1}))
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Minutes: 4, Reviews: 1, Failed: 1, Activities: []string{"Review"}}))
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Activities: []string{"Review", "Reading"}}))

	days, err := s.Days(ctx)
	must(t, err)
	want := []domain.Day{{Date: nov9, Minutes: 4, Reviews: 2, Correct: 1, Failed: 1, Activities: []string{"Review", "Reading"}}}
	if !reflect.DeepEqual(days, want) {
		t.Errorf("Days = %+v, want %+v", days, want)
	}
}

func TestRecordStudyTakesBackAnUndoneGrade(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: 1, Correct: 1}))

	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: -1, Correct: -1}))
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: -1, Failed: -1})) // never below zero

	days, _ := s.Days(ctx)
	if d := days[0]; d.Reviews != 0 || d.Correct != 0 || d.Failed != 0 {
		t.Errorf("after undo the day is %+v, want zero counts", d)
	}
}

func TestDaysAreOrderedByDate(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: 1}))
	must(t, s.RecordStudy(ctx, domain.Day{Date: aug14, Reviews: 1}))

	days, _ := s.Days(ctx)
	if len(days) != 2 || days[0].Date != aug14 || days[1].Date != nov9 {
		t.Errorf("Days = %+v, want aug14 then nov9", days)
	}
}

func TestMergeDaysIsIdempotent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	must(t, s.RecordStudy(ctx, domain.Day{Date: aug14, Reviews: 3, Activities: []string{"Review"}}))
	history := []domain.Day{
		{Date: aug14, Activities: []string{"Lesson 1 (Greetings)", "Homework review"}},
		{Date: aug14.AddDays(-1), Activities: []string{"Session 1"}},
	}

	for range 2 {
		added, err := s.MergeDays(ctx, history)
		must(t, err)
		_ = added
	}

	days, _ := s.Days(ctx)
	want := []domain.Day{
		{Date: aug14.AddDays(-1), Activities: []string{"Session 1"}},
		{Date: aug14, Reviews: 3, Activities: []string{"Review", "Lesson 1 (Greetings)", "Homework review"}},
	}
	if !reflect.DeepEqual(days, want) {
		t.Errorf("Days = %+v\nwant %+v", days, want)
	}
}

func TestMergeDaysCountsNewDays(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	history := []domain.Day{{Date: aug14, Activities: []string{"Lesson 1"}}}

	first, err := s.MergeDays(ctx, history)
	must(t, err)
	second, err := s.MergeDays(ctx, history)
	must(t, err)

	if first != 1 || second != 0 {
		t.Errorf("MergeDays added %d then %d, want 1 then 0", first, second)
	}
}

func TestProfileDefaultsUntilSaved(t *testing.T) {
	s := openTemp(t)

	p, err := s.Profile(context.Background())

	if err != nil || !reflect.DeepEqual(p, domain.NewProfile()) {
		t.Errorf("Profile = %+v, %v; want the default profile", p, err)
	}
}

func TestSaveProfileRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	want := domain.Profile{Name: "Kevin", Level: domain.A2, Started: aug14, DailyGoalMinutes: 25}

	must(t, s.SaveProfile(ctx, want))
	must(t, s.SaveProfile(ctx, want)) // saving twice keeps one profile

	got, err := s.Profile(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Profile = %+v, %v; want %+v", got, err, want)
	}
}

func TestProgressSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	ctx := context.Background()
	s, err := Open(path)
	must(t, err)
	must(t, s.RecordStudy(ctx, domain.Day{Date: nov9, Reviews: 2}))
	must(t, s.SaveProfile(ctx, domain.Profile{Name: "Kevin", Level: domain.A1, DailyGoalMinutes: 20}))
	s.Close()

	s, err = Open(path)
	must(t, err)
	defer s.Close()
	days, _ := s.Days(ctx)
	p, _ := s.Profile(ctx)
	if len(days) != 1 || days[0].Reviews != 2 || p.Name != "Kevin" {
		t.Errorf("after reopen days %+v profile %+v", days, p)
	}
}

func TestMigrationThreeKeepsCards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	due := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	reviewed := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	openV1(t, path, due, reviewed)

	s, err := Open(path)
	must(t, err)
	defer s.Close()

	var version int
	must(t, s.db.QueryRow(`PRAGMA user_version`).Scan(&version))
	if version != len(migrations) || version < 3 {
		t.Errorf("user_version = %d, want %d", version, len(migrations))
	}
	cards, err := s.ListCards(context.Background(), domain.Recognition)
	must(t, err)
	if len(cards) == 0 || !cards[0].Due.Equal(due) {
		t.Errorf("cards after migration 3: %+v", cards)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
