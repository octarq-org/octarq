package models

import "testing"

// TestInStatusSet pins the closed-vocabulary check the API validates every
// inbound status and priority against.
func TestInStatusSet(t *testing.T) {
	t.Parallel()

	for _, v := range WorkforceTaskStatuses {
		if !InStatusSet(v, WorkforceTaskStatuses) {
			t.Errorf("%q must belong to its own vocabulary", v)
		}
	}
	for _, v := range []string{"", "all", "blocked", "DONE", "done "} {
		if InStatusSet(v, WorkforceTaskStatuses) {
			t.Errorf("%q must not be accepted as a task status", v)
		}
	}
	if InStatusSet("any", nil) {
		t.Error("an empty vocabulary accepts nothing")
	}
	if !InStatusSet("urgent", WorkforceTaskPriorities) {
		t.Error("urgent must be a supported priority")
	}
}
