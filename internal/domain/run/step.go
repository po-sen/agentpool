package run

import "time"

// Step describes one coarse execution step in a run.
type Step struct {
	Name      string
	Status    Status
	Message   string
	StartedAt time.Time
	EndedAt   time.Time
}
