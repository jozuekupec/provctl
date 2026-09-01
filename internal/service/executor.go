package service

import (
	"provctl/internal/audit"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

func productionExecutor(repository *sqlite.Repository) plan.Executor {
	return plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}, Audit: audit.FileWriter{Path: meta.AuditLog}}
}
