package system

// Identity exposes process identity without coupling services to the OS package.
type Identity interface {
	EUID() int
}
