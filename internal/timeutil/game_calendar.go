package timeutil

import "errors"

const (
	RealSecondsPerGameDay = 28_800
	HoursPerDay           = 24
	DaysPerMonth          = 30
	MonthsPerYear         = 12
)

var errNegativeCalendarRuntime = errors.New("runtime seconds must not be negative")

// GameCalendar describes the world date and time derived from server runtime.
// Date fields start at one; elapsed-period indexes start at zero.
type GameCalendar struct {
	Year       int64
	Month      int64
	Day        int64
	Hour       int64
	Minute     int64
	Second     int64
	DayIndex   int64
	MonthIndex int64
	YearIndex  int64
	DayPhase   float64
}

// GameCalendarFromRuntime converts nonnegative whole runtime seconds without
// multiplying the total runtime or narrowing it to a time.Duration.
func GameCalendarFromRuntime(runtimeSeconds int64) (GameCalendar, error) {
	if runtimeSeconds < 0 {
		return GameCalendar{}, errNegativeCalendarRuntime
	}

	dayIndex := runtimeSeconds / RealSecondsPerGameDay
	runtimeSecondOfDay := runtimeSeconds % RealSecondsPerGameDay
	gameSecondOfDay := runtimeSecondOfDay * (HoursPerDay * 60 * 60) / RealSecondsPerGameDay
	monthIndex := dayIndex / DaysPerMonth
	yearIndex := monthIndex / MonthsPerYear

	return GameCalendar{
		Year:       yearIndex + 1,
		Month:      monthIndex%MonthsPerYear + 1,
		Day:        dayIndex%DaysPerMonth + 1,
		Hour:       gameSecondOfDay / 3600,
		Minute:     gameSecondOfDay / 60 % 60,
		Second:     gameSecondOfDay % 60,
		DayIndex:   dayIndex,
		MonthIndex: monthIndex,
		YearIndex:  yearIndex,
		DayPhase:   float64(runtimeSecondOfDay) / RealSecondsPerGameDay,
	}, nil
}
