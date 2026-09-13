package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule defines when a cron job should run next.
type Schedule interface {
	Next(from time.Time) time.Time
}

// EverySchedule runs a task repeatedly at fixed duration intervals.
type EverySchedule struct {
	Duration time.Duration
}

func (s EverySchedule) Next(from time.Time) time.Time {
	if s.Duration <= 0 {
		return time.Time{}
	}
	return from.Add(s.Duration)
}

// CronSchedule represents a parsed standard 5-field or 6-field (with seconds) cron expression.
type CronSchedule struct {
	seconds       uint64 // bits 0-59
	minutes       uint64 // bits 0-59
	hours         uint64 // bits 0-23
	daysOfMonth   uint64 // bits 1-31
	months        uint64 // bits 1-12
	daysOfWeek    uint64 // bits 0-6 (0=Sun, 6=Sat)
	domRestricted bool
	dowRestricted bool
}

var monthNames = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var dayOfWeekNames = map[string]int{
	"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
}

// ParseStandard parses a cron expression (5 fields, 6 fields with seconds, or descriptors).
func ParseStandard(spec string) (Schedule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("cron: empty spec")
	}

	// Handle descriptors
	if strings.HasPrefix(spec, "@") {
		lower := strings.ToLower(spec)
		if strings.HasPrefix(lower, "@every ") {
			durationStr := strings.TrimSpace(spec[len("@every "):])
			d, err := time.ParseDuration(durationStr)
			if err != nil {
				return nil, fmt.Errorf("cron: invalid duration in %q: %w", spec, err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("cron: @every duration must be positive: %s", durationStr)
			}
			return EverySchedule{Duration: d}, nil
		}

		switch lower {
		case "@hourly":
			return parseFields([]string{"0", "*", "*", "*", "*"})
		case "@daily", "@midnight":
			return parseFields([]string{"0", "0", "*", "*", "*"})
		case "@weekly":
			return parseFields([]string{"0", "0", "*", "*", "0"})
		case "@monthly":
			return parseFields([]string{"0", "0", "1", "*", "*"})
		case "@yearly", "@annually":
			return parseFields([]string{"0", "0", "1", "1", "*"})
		default:
			return nil, fmt.Errorf("cron: unrecognized descriptor: %q", spec)
		}
	}

	fields := strings.Fields(spec)
	if len(fields) == 5 {
		return parseFields(fields)
	} else if len(fields) == 6 {
		return parseFields6(fields)
	}
	return nil, fmt.Errorf("cron: expected 5 or 6 fields, got %d in %q", len(fields), spec)
}

func parseFields(fields []string) (*CronSchedule, error) {
	// 5 fields: minute hour dom month dow
	// seconds defaults to bit 0 set (second = 0)
	sched := &CronSchedule{
		seconds: 1 << 0,
	}

	min, err := parseField(fields[0], 0, 59, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: minute field: %w", err)
	}
	sched.minutes = min

	hr, err := parseField(fields[1], 0, 23, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: hour field: %w", err)
	}
	sched.hours = hr

	dom, err := parseField(fields[2], 1, 31, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: day of month field: %w", err)
	}
	sched.daysOfMonth = dom
	sched.domRestricted = fields[2] != "*" && fields[2] != "?"

	mon, err := parseField(fields[3], 1, 12, monthNames)
	if err != nil {
		return nil, fmt.Errorf("cron: month field: %w", err)
	}
	sched.months = mon

	dow, err := parseDayOfWeekField(fields[4])
	if err != nil {
		return nil, fmt.Errorf("cron: day of week field: %w", err)
	}
	sched.daysOfWeek = dow
	sched.dowRestricted = fields[4] != "*" && fields[4] != "?"

	return sched, nil
}

func parseFields6(fields []string) (*CronSchedule, error) {
	// 6 fields: second minute hour dom month dow
	sched := &CronSchedule{}

	sec, err := parseField(fields[0], 0, 59, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: second field: %w", err)
	}
	sched.seconds = sec

	min, err := parseField(fields[1], 0, 59, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: minute field: %w", err)
	}
	sched.minutes = min

	hr, err := parseField(fields[2], 0, 23, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: hour field: %w", err)
	}
	sched.hours = hr

	dom, err := parseField(fields[3], 1, 31, nil)
	if err != nil {
		return nil, fmt.Errorf("cron: day of month field: %w", err)
	}
	sched.daysOfMonth = dom
	sched.domRestricted = fields[3] != "*" && fields[3] != "?"

	mon, err := parseField(fields[4], 1, 12, monthNames)
	if err != nil {
		return nil, fmt.Errorf("cron: month field: %w", err)
	}
	sched.months = mon

	dow, err := parseDayOfWeekField(fields[5])
	if err != nil {
		return nil, fmt.Errorf("cron: day of week field: %w", err)
	}
	sched.daysOfWeek = dow
	sched.dowRestricted = fields[5] != "*" && fields[5] != "?"

	return sched, nil
}

func parseField(expr string, min, max int, names map[string]int) (uint64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty field")
	}

	var bits uint64
	parts := strings.Split(expr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, fmt.Errorf("empty item in list %q", expr)
		}

		var rangeStr string
		step := 1

		if slashIdx := strings.Index(part, "/"); slashIdx != -1 {
			stepStr := part[slashIdx+1:]
			var err error
			step, err = strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return 0, fmt.Errorf("invalid step %q in %q", stepStr, part)
			}
			rangeStr = part[:slashIdx]
		} else {
			rangeStr = part
		}

		var start, end int
		if rangeStr == "*" || rangeStr == "?" {
			start = min
			end = max
		} else if dashIdx := strings.Index(rangeStr, "-"); dashIdx != -1 {
			startStr := rangeStr[:dashIdx]
			endStr := rangeStr[dashIdx+1:]
			s, err := parseVal(startStr, min, max, names)
			if err != nil {
				return 0, err
			}
			e, err := parseVal(endStr, min, max, names)
			if err != nil {
				return 0, err
			}
			if s > e {
				return 0, fmt.Errorf("range start %d greater than end %d in %q", s, e, part)
			}
			start = s
			end = e
		} else {
			val, err := parseVal(rangeStr, min, max, names)
			if err != nil {
				return 0, err
			}
			if strings.Contains(part, "/") {
				start = val
				end = max
			} else {
				start = val
				end = val
			}
		}

		for i := start; i <= end; i += step {
			bits |= 1 << uint(i)
		}
	}

	if bits == 0 {
		return 0, fmt.Errorf("no valid values in %q", expr)
	}
	return bits, nil
}

func parseDayOfWeekField(expr string) (uint64, error) {
	bits, err := parseField(expr, 0, 7, dayOfWeekNames)
	if err != nil {
		return 0, err
	}
	// Day 7 is also Sunday (0)
	if (bits & (1 << 7)) != 0 {
		bits |= (1 << 0)
		bits &^= (1 << 7)
	}
	return bits, nil
}

func parseVal(s string, min, max int, names map[string]int) (int, error) {
	s = strings.TrimSpace(s)
	if names != nil {
		if val, ok := names[strings.ToUpper(s)]; ok {
			return val, nil
		}
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", s)
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value %d out of range [%d, %d]", val, min, max)
	}
	return val, nil
}

func (s *CronSchedule) matchDay(t time.Time) bool {
	domMatch := (s.daysOfMonth & (1 << uint(t.Day()))) != 0
	dowMatch := (s.daysOfWeek & (1 << uint(t.Weekday()))) != 0

	if s.domRestricted && s.dowRestricted {
		return domMatch || dowMatch
	}
	if s.domRestricted {
		return domMatch
	}
	if s.dowRestricted {
		return dowMatch
	}
	return true
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Next returns the earliest time after `from` that satisfies the cron schedule.
// If no match is found within 5 years, returns zero time.
func (s *CronSchedule) Next(from time.Time) time.Time {
	t := from.Truncate(time.Second).Add(time.Second)
	loc := from.Location()
	if loc == nil {
		loc = time.Local
	}
	t = t.In(loc)

	limitYear := t.Year() + 5

	for t.Year() <= limitYear {
		// Month match
		if (s.months & (1 << uint(t.Month()))) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}

		// Day match
		if t.Day() > daysInMonth(t.Year(), t.Month()) || !s.matchDay(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}

		// Hour match
		if (s.hours & (1 << uint(t.Hour()))) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}

		// Minute match
		if (s.minutes & (1 << uint(t.Minute()))) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()+1, 0, 0, loc)
			continue
		}

		// Second match
		if (s.seconds & (1 << uint(t.Second()))) == 0 {
			t = t.Add(time.Second)
			continue
		}

		return t
	}

	return time.Time{}
}
