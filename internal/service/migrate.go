package service

import (
	"context"

	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
)

// MigrationRuntime owns the state repository while an explicit schema
// migration is performed.
type MigrationRuntime struct {
	Repository *sqlite.Repository
}

func NewProductionMigrationRuntime(ctx context.Context) (*MigrationRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &MigrationRuntime{Repository: repository}, nil
}

func (runtime *MigrationRuntime) Close() error { return runtime.Repository.Close() }
