package system

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
)

// CommandUsers makes account changes through explicit user-management commands.
type CommandUsers struct {
	Commander Commander
}

func (users CommandUsers) Lookup(name string) (*user.User, error)  { return user.Lookup(name) }
func (users CommandUsers) LookupID(uid string) (*user.User, error) { return user.LookupId(uid) }

func (users CommandUsers) Create(ctx context.Context, options CreateUserOptions) error {
	args := []string{"--uid", strconv.Itoa(options.UID), "--home", options.Home, "--shell", options.Shell}
	if options.UserGroup {
		args = append(args, "--user-group")
	} else {
		args = append(args, "--gid", strconv.Itoa(options.GID))
	}
	if options.System {
		args = append(args, "--system")
	}
	if options.NoCreateHome {
		args = append(args, "--no-create-home")
	} else {
		args = append(args, "--create-home")
	}
	args = append(args, options.Name)
	if _, err := users.Commander.Run(ctx, "/usr/sbin/useradd", args...); err != nil {
		return fmt.Errorf("create user %q: %w", options.Name, err)
	}
	return nil
}

func (users CommandUsers) SetShell(ctx context.Context, name, shell string) error {
	return users.run(ctx, "/usr/sbin/usermod", "--shell", shell, name)
}

func (users CommandUsers) LockPassword(ctx context.Context, name string) error {
	return users.run(ctx, "/usr/sbin/usermod", "--lock", name)
}

func (users CommandUsers) SetPassword(ctx context.Context, name, password string) error {
	return users.runWithStdin(ctx, name+":"+password+"\n", "/usr/sbin/chpasswd")
}

func (users CommandUsers) Delete(ctx context.Context, name string, removeHome bool) error {
	args := []string{}
	if removeHome {
		args = append(args, "--remove")
	}
	args = append(args, name)
	return users.run(ctx, "/usr/sbin/userdel", args...)
}

func (users CommandUsers) run(ctx context.Context, binary string, args ...string) error {
	if _, err := users.Commander.Run(ctx, binary, args...); err != nil {
		return fmt.Errorf("run %s: %w", binary, err)
	}
	return nil
}

func (users CommandUsers) runWithStdin(ctx context.Context, input, binary string, args ...string) error {
	if _, err := users.Commander.RunWithStdin(ctx, strings.NewReader(input), binary, args...); err != nil {
		return fmt.Errorf("run %s: %w", binary, err)
	}
	return nil
}
