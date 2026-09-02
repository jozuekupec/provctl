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

// BackupMetadata is the non-secret manifest stored in metadata.json.
type BackupMetadata struct {
	FormatVersion  int           `json:"format_version"`
	ProvctlVersion string        `json:"provctl_version"`
	CreatedAt      time.Time     `json:"created_at"`
	Subscription   Subscription  `json:"subscription"`
	Websites       []Website     `json:"websites"`
	Databases      []Database    `json:"databases"`
	CronJobs       []CronJob     `json:"cron_jobs"`
	SSHKeys        []SSHKey      `json:"ssh_keys"`
	Certificates   []Certificate `json:"certificates"`
}
