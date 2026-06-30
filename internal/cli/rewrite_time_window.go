package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var commitTimeWindowRe = regexp.MustCompile(`^([0-2][0-9]):([0-5][0-9])-([0-2][0-9]):([0-5][0-9])$`)

type commitTimeWindow struct {
	StartMinute int
	EndMinute   int
	Text        string
}

type commitTimeSchedule struct {
	windows [7]commitTimeWindow
	set     [7]bool
	Text    string
}

func parseCommitTimeWindow(value string) (commitTimeWindow, error) {
	value = strings.TrimSpace(value)
	matches := commitTimeWindowRe.FindStringSubmatch(value)
	if matches == nil {
		return commitTimeWindow{}, fmt.Errorf("window must be in HH:MM-HH:MM format")
	}
	startHour, _ := strconv.Atoi(matches[1])
	startMinute, _ := strconv.Atoi(matches[2])
	endHour, _ := strconv.Atoi(matches[3])
	endMinute, _ := strconv.Atoi(matches[4])
	if startHour > 23 || endHour > 23 {
		return commitTimeWindow{}, fmt.Errorf("window hours must be from 00 through 23")
	}
	start := startHour*60 + startMinute
	end := endHour*60 + endMinute
	if start >= end {
		return commitTimeWindow{}, fmt.Errorf("window start must be before window end")
	}
	return commitTimeWindow{StartMinute: start, EndMinute: end, Text: value}, nil
}

func parseCommitTimeSchedule(value string) (commitTimeSchedule, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return commitTimeSchedule{}, fmt.Errorf("window is required")
	}
	if !strings.Contains(value, "=") {
		window, err := parseCommitTimeWindow(value)
		if err != nil {
			return commitTimeSchedule{}, err
		}
		schedule := commitTimeSchedule{Text: window.Text}
		for day := time.Sunday; day <= time.Saturday; day++ {
			schedule.windows[day] = window
			schedule.set[day] = true
		}
		return schedule, nil
	}

	schedule := commitTimeSchedule{Text: value}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return commitTimeSchedule{}, fmt.Errorf("window schedule contains an empty entry")
		}
		selector, rawWindow, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(selector) == "" || strings.TrimSpace(rawWindow) == "" {
			return commitTimeSchedule{}, fmt.Errorf("window schedule entries must be day=HH:MM-HH:MM")
		}
		window, err := parseCommitTimeWindow(rawWindow)
		if err != nil {
			return commitTimeSchedule{}, err
		}
		days, err := parseCommitTimeScheduleDays(selector)
		if err != nil {
			return commitTimeSchedule{}, err
		}
		for _, day := range days {
			if schedule.set[day] {
				return commitTimeSchedule{}, fmt.Errorf("window schedule assigns %s more than once", strings.ToLower(day.String()[:3]))
			}
			schedule.windows[day] = window
			schedule.set[day] = true
		}
	}
	if schedule.empty() {
		return commitTimeSchedule{}, fmt.Errorf("window schedule must assign at least one day")
	}
	return schedule, nil
}

func parseCommitTimeScheduleDays(value string) ([]time.Weekday, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil, fmt.Errorf("window schedule day is required")
	}
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return nil, fmt.Errorf("window schedule day range %q is invalid", value)
	}
	start, ok := commitTimeScheduleDay(parts[0])
	if !ok {
		return nil, fmt.Errorf("unknown window schedule day %q", parts[0])
	}
	if len(parts) == 1 {
		return []time.Weekday{start}, nil
	}
	end, ok := commitTimeScheduleDay(parts[1])
	if !ok {
		return nil, fmt.Errorf("unknown window schedule day %q", parts[1])
	}
	if start > end {
		return nil, fmt.Errorf("window schedule day range %q must not wrap", value)
	}
	days := make([]time.Weekday, 0, int(end-start)+1)
	for day := start; day <= end; day++ {
		days = append(days, day)
	}
	return days, nil
}

func commitTimeScheduleDay(value string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sun":
		return time.Sunday, true
	case "mon":
		return time.Monday, true
	case "tue":
		return time.Tuesday, true
	case "wed":
		return time.Wednesday, true
	case "thu":
		return time.Thursday, true
	case "fri":
		return time.Friday, true
	case "sat":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func (s commitTimeSchedule) windowForDay(day int64, tzOffset string) (commitTimeWindow, bool) {
	weekday := time.Unix(day, 0).In(locationForTimezoneOffset(tzOffset)).Weekday()
	return s.windows[weekday], s.set[weekday]
}

func (s commitTimeSchedule) empty() bool {
	for _, ok := range s.set {
		if ok {
			return false
		}
	}
	return true
}

func commitTimeWindowBounds(day int64, window commitTimeWindow) (int64, int64) {
	start := day + int64(window.StartMinute)*60
	end := day + int64(window.EndMinute)*60 - 1
	if end < start {
		end = start
	}
	return start, end
}

func plannedEpochsForExplicitWindow(day int64, count int, startEpoch, endEpoch int64, window commitTimeWindow) []int64 {
	if count <= 0 {
		return nil
	}
	windowStart, windowEnd := commitTimeWindowBounds(day, window)
	start := maxInt64(windowStart, startEpoch)
	end := minInt64(windowEnd, endEpoch)
	if end < start {
		nearest := clampInt64(windowStart, startEpoch, endEpoch)
		start = nearest
		end = nearest
	}
	return evenlyDistributedEpochs(count, start, end)
}

func evenlyDistributedEpochs(count int, start, end int64) []int64 {
	if count <= 0 {
		return nil
	}
	if end < start {
		end = start
	}
	timestamps := make([]int64, count)
	if count == 1 {
		timestamps[0] = start + (end-start)/2
		return timestamps
	}
	available := end - start + 1
	if available <= 1 {
		for i := range timestamps {
			timestamps[i] = start
		}
		return timestamps
	}
	if int64(count) <= available {
		span := available - 1
		for i := range timestamps {
			timestamps[i] = start + int64(i)*span/int64(count-1)
		}
		return timestamps
	}
	for i := range timestamps {
		timestamps[i] = start + int64(i)*available/int64(count)
	}
	return timestamps
}

func epochInCommitTimeWindow(epoch int64, tzOffset string, window commitTimeWindow) bool {
	day := floorDayInOffset(epoch, tzOffset)
	start, end := commitTimeWindowBounds(day, window)
	return epoch >= start && epoch <= end
}
