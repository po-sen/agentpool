package approval

// Decision captures a human approval outcome.
type Decision string

const (
	// DecisionApproved allows a paused run to continue.
	DecisionApproved Decision = "approved"
	// DecisionRejected denies a paused run from continuing.
	DecisionRejected Decision = "rejected"
)
