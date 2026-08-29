package system

import "os"

// OSIdentity reports the effective identity of the current process.
type OSIdentity struct{}

func (OSIdentity) EUID() int { return os.Geteuid() }
