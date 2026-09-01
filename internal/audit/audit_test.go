package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWriter_WritesSecretFreeJSONLine(t *testing.T) {
	t.Setenv("SUDO_USER", "operator")
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writer := FileWriter{Path: path}
	if err := writer.Write(context.Background(), Entry{Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Action: "subscription.create", Target: "acme", Status: "ok", DurationMS: 42, OperationID: 7}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry Entry
	if err := json.Unmarshal(contents, &entry); err != nil {
		t.Fatalf("audit JSON = %q: %v", contents, err)
	}
	if entry.Actor != "operator" || entry.Action != "subscription.create" || entry.OperationID != 7 {
		t.Errorf("entry = %#v", entry)
	}
}
