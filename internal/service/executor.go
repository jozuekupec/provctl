package service

import (
	"context"
	"provctl/internal/audit"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
	"time"
)

func productionExecutor(repository *sqlite.Repository) plan.Executor {
	return plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}, Audit: audit.FileWriter{Path: meta.AuditLog}}
}

func writeDirectAudit(writer audit.Writer, action, target string, started time.Time, operationErr error) {
	if writer == nil {
		return
	}
	status := "ok"
	if operationErr != nil {
		status = "failed"
	}
	_ = writer.Write(context.Background(), audit.Entry{Timestamp: started, Action: action, Target: target, Status: status, DurationMS: time.Since(started).Milliseconds()})
}
