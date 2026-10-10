package timeutil

import (
	"math"
	"strconv"
	"testing"
)

func TestGameCalendarFromRuntimeBoundaries(t *testing.T) {
	tests := []struct {
		runtime int64
		want    GameCalendar
	}{
		{0, GameCalendar{Year: 1, Month: 1, Day: 1}},
		{19, GameCalendar{Year: 1, Month: 1, Day: 1, Second: 57, DayPhase: 19.0 / 28800}},
		{20, GameCalendar{Year: 1, Month: 1, Day: 1, Minute: 1, DayPhase: 20.0 / 28800}},
		{1199, GameCalendar{Year: 1, Month: 1, Day: 1, Minute: 59, Second: 57, DayPhase: 1199.0 / 28800}},
		{1200, GameCalendar{Year: 1, Month: 1, Day: 1, Hour: 1, DayPhase: 1200.0 / 28800}},
		{28799, GameCalendar{Year: 1, Month: 1, Day: 1, Hour: 23, Minute: 59, Second: 57, DayPhase: 28799.0 / 28800}},
		{28800, GameCalendar{Year: 1, Month: 1, Day: 2, DayIndex: 1}},
		{364686, GameCalendar{Year: 1, Month: 1, Day: 13, Hour: 15, Minute: 54, Second: 18, DayIndex: 12, DayPhase: 19086.0 / 28800}},
		{863999, GameCalendar{Year: 1, Month: 1, Day: 30, Hour: 23, Minute: 59, Second: 57, DayIndex: 29, DayPhase: 28799.0 / 28800}},
		{864000, GameCalendar{Year: 1, Month: 2, Day: 1, DayIndex: 30, MonthIndex: 1}},
		{10367999, GameCalendar{Year: 1, Month: 12, Day: 30, Hour: 23, Minute: 59, Second: 57, DayIndex: 359, MonthIndex: 11, DayPhase: 28799.0 / 28800}},
		{10368000, GameCalendar{Year: 2, Month: 1, Day: 1, DayIndex: 360, MonthIndex: 12, YearIndex: 1}},
		{math.MaxInt64, GameCalendar{Year: 889599926395, Month: 3, Day: 2, Hour: 22, Minute: 30, Second: 21, DayIndex: 320255973501901, MonthIndex: 10675199116730, YearIndex: 889599926394, DayPhase: 27007.0 / 28800}},
	}
	for _, test := range tests {
		t.Run(strconv.FormatInt(test.runtime, 10), func(t *testing.T) {
			got, err := GameCalendarFromRuntime(test.runtime)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("calendar = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestGameCalendarFromRuntimeRejectsNegative(t *testing.T) {
	for _, runtime := range []int64{-1, math.MinInt64} {
		if got, err := GameCalendarFromRuntime(runtime); err == nil || got != (GameCalendar{}) {
			t.Fatalf("runtime %d: calendar = %+v, error = %v; want zero calendar and error", runtime, got, err)
		}
	}
}
