package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var commitTimeWindowRe = regexp.MustCompile(`^([0-2][0-9]):([0-5][0-9])-([0-2][0-9]):([0-5][0-9])$`)

type commitTimeWindow struct {
	StartMinute int
	EndMinute   int
	Text        string
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
