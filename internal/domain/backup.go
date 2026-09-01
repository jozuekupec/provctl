package domain

import "time"

// Backup is the persisted state of one subscription archive. It contains no
// archive contents or credentials.
type Backup struct {
	ID             int64
	SubscriptionID int64
	Path           string
	SizeBytes      int64
	Status         string
	StartedAt      time.Time
	FinishedAt     time.Time
}
