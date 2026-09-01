// Package audit writes the narrow, secret-free operational audit trail.
package audit

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

type Entry struct {
	Timestamp   time.Time `json:"ts"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	Target      string    `json:"target"`
	Status      string    `json:"status"`
	DurationMS  int64     `json:"duration_ms"`
	OperationID int64     `json:"operation_id,omitempty"`
}

type Writer interface {
	Write(context.Context, Entry) error
}

type FileWriter struct{ Path string }

func (writer FileWriter) Write(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry.Timestamp = entry.Timestamp.UTC()
	entry.Actor = actor()
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(writer.Path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(encoded, '\n'))
	return err
}

func actor() string {
	for _, name := range []string{"SUDO_USER", "USER"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return "root"
}
