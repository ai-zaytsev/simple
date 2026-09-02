package main

import (
	"testing"
	"time"
)

func msk(hour, minute int) time.Time {
	return time.Date(2026, 9, 2, hour, minute, 0, 0, time.FixedZone("MSK", 3*60*60))
}

func TestTheDailyCheckIsOwedOnceAfterSix(t *testing.T) {
	yesterday := msk(6, 5).AddDate(0, 0, -1)

	if dueForCheck(msk(5, 59), &yesterday) {
		t.Error("before six the check is not owed")
	}
	if !dueForCheck(msk(6, 0), &yesterday) {
		t.Error("at six it is owed")
	}
	if !dueForCheck(msk(23, 0), &yesterday) {
		t.Error("still owed later in the day if it never ran")
	}

	// Ran this morning. Not owed again today, however long the day gets - the
	// stage says one full check a day and no polling.
	thisMorning := msk(6, 1)
	if dueForCheck(msk(12, 0), &thisMorning) {
		t.Error("a check already made today must not be repeated")
	}
	if dueForCheck(msk(23, 59), &thisMorning) {
		t.Error("still not repeated at the end of the day")
	}
}

func TestARestartAtSixDoesNotSkipTheDay(t *testing.T) {
	// A daily timer that restarts with the process would push the check to
	// tomorrow, and tomorrow is a whole day of selling with nothing checked.
	yesterday := msk(6, 5).AddDate(0, 0, -1)
	if !dueForCheck(msk(6, 30), &yesterday) {
		t.Fatal("a restart just after six must still run the check")
	}
}

func TestNeverCheckedIsOwedAsSoonAsSixHasPassed(t *testing.T) {
	if !dueForCheck(msk(6, 0), nil) {
		t.Error("a deployment that has never checked is owed one")
	}
	if dueForCheck(msk(4, 0), nil) {
		t.Error("but not before six: the day's check belongs to the day")
	}
}
