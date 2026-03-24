package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func parseDailyClock(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format: %s", value)
	}
	hour, err := time.Parse("15:04", fmt.Sprintf("%s:%s", parts[0], parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return hour.Hour(), hour.Minute(), nil
}

func nextRunAt(clock string, now time.Time) time.Time {
	hour, minute, err := parseDailyClock(clock)
	if err != nil {
		hour, minute = 0, 0
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func runDailyTask(taskName string, getClock func() string, isEnabled func() bool, run func()) {
	go func() {
		for {
			now := time.Now()
			next := nextRunAt(getClock(), now)
			wait := time.Until(next)
			common.SysLog(fmt.Sprintf("[%s] next run at %s", taskName, next.Format(time.RFC3339)))
			time.Sleep(wait)
			if isEnabled() {
				run()
			}
		}
	}()
}
