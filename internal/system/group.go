package system

import (
	"fmt"
	"os/user"
	"strconv"
)

// GroupLookup resolves a system group without exposing os/user to services.
type GroupLookup interface {
	LookupGroupID(name string) (int, error)
}

// OSGroupLookup resolves groups from the host account database.
type OSGroupLookup struct{}

func (OSGroupLookup) LookupGroupID(name string) (int, error) {
	group, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, fmt.Errorf("parse group %q id %q: %w", name, group.Gid, err)
	}
	return id, nil
}
