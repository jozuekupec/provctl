package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// SSHKeyStore persists public SSH keys and their subscription association.
type SSHKeyStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListSSHKeys(context.Context, int64) ([]domain.SSHKey, error)
	CreateSSHKey(context.Context, domain.SSHKey) (int64, error)
	DeleteSSHKey(context.Context, int64, string) error
	UpdateSSHAccess(context.Context, int64, string) error
}

// SSHService makes authorized_keys a generated artifact of SQLite state.
type SSHService struct {
	FS       system.FS
	Commands system.Commander
	Users    system.Users
	Store    SSHKeyStore
	Executor plan.Executor
}

// SSHRuntime owns the SQLite connection used by SSH-key commands.
type SSHRuntime struct {
	Service    SSHService
	repository *sqlite.Repository
}

func NewProductionSSHRuntime(ctx context.Context) (*SSHRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &SSHRuntime{Service: SSHService{FS: system.OSFS{}, Commands: commander, Users: system.CommandUsers{Commander: commander}, Store: repository, Executor: productionExecutor(repository)}, repository: repository}, nil
}

func NewReadOnlySSHRuntime(ctx context.Context) (*SSHRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &SSHRuntime{Service: SSHService{Store: repository}, repository: repository}, nil
}

func (runtime *SSHRuntime) Close() error { return runtime.repository.Close() }

func (service SSHService) List(ctx context.Context, subscriptionName string) ([]domain.SSHKey, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return nil, err
	}
	if service.Store == nil {
		return nil, fmt.Errorf("SSH key store is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	return service.Store.ListSSHKeys(ctx, subscription.ID)
}

func (service SSHService) Add(ctx context.Context, subscriptionName, publicKey string) (int64, error) {
	operation, err := service.PrepareAdd(ctx, subscriptionName, publicKey)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// AddFromFile reads one public key through the service filesystem seam.
func (service SSHService) AddFromFile(ctx context.Context, subscriptionName, path string) (int64, error) {
	if service.FS == nil {
		return 0, fmt.Errorf("filesystem is required")
	}
	contents, err := service.FS.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read SSH public key: %w", err)
	}
	return service.Add(ctx, subscriptionName, string(contents))
}

func (service SSHService) PrepareAdd(ctx context.Context, subscriptionName, publicKey string) (plan.Plan, error) {
	subscription, keys, err := service.subscriptionKeys(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	fingerprint, comment, normalized, err := service.validatePublicKey(ctx, publicKey)
	if err != nil {
		return plan.Plan{}, err
	}
	for _, key := range keys {
		if key.Fingerprint == fingerprint {
			return plan.Plan{}, fmt.Errorf("SSH key %q already exists", fingerprint)
		}
	}
	key := domain.SSHKey{SubscriptionID: subscription.ID, Comment: comment, Fingerprint: fingerprint, PublicKey: normalized}
	desired := append(append([]domain.SSHKey(nil), keys...), key)
	var undoFile func(context.Context) error
	steps := []plan.Step{{Name: "write generated authorized_keys", Preview: "write " + authorizedKeysPath(subscription), Do: func(ctx context.Context) error {
		var err error
		undoFile, err = service.writeAuthorizedKeys(subscription, desired)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoFile == nil {
			return nil
		}
		return undoFile(ctx)
	}}, {Name: "record SSH key in SQLite", Preview: "insert SSH fingerprint " + fingerprint, Do: func(ctx context.Context) error {
		id, err := service.Store.CreateSSHKey(ctx, key)
		key.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteSSHKey(ctx, subscription.ID, fingerprint) }}}
	return plan.Plan{Action: "ssh.key.add", Target: subscription.Name + "/" + fingerprint, Steps: steps}, nil
}

func (service SSHService) Remove(ctx context.Context, subscriptionName, fingerprint string) (int64, error) {
	operation, err := service.PrepareRemove(ctx, subscriptionName, fingerprint)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service SSHService) PrepareRemove(ctx context.Context, subscriptionName, fingerprint string) (plan.Plan, error) {
	subscription, keys, err := service.subscriptionKeys(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var removed domain.SSHKey
	desired := make([]domain.SSHKey, 0, len(keys)-1)
	for _, key := range keys {
		if key.Fingerprint == fingerprint {
			removed = key
			continue
		}
		desired = append(desired, key)
	}
	if removed.ID == 0 {
		return plan.Plan{}, fmt.Errorf("SSH key %q not found", fingerprint)
	}
	var undoFile func(context.Context) error
	steps := []plan.Step{{Name: "write generated authorized_keys", Preview: "write " + authorizedKeysPath(subscription), Do: func(ctx context.Context) error {
		var err error
		undoFile, err = service.writeAuthorizedKeys(subscription, desired)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoFile == nil {
			return nil
		}
		return undoFile(ctx)
	}}, {Name: "remove SSH key from SQLite", Preview: "delete SSH fingerprint " + fingerprint, Do: func(ctx context.Context) error {
		return service.Store.DeleteSSHKey(ctx, subscription.ID, fingerprint)
	}, Undo: func(ctx context.Context) error {
		_, err := service.Store.CreateSSHKey(ctx, removed)
		return err
	}}}
	return plan.Plan{Action: "ssh.key.remove", Target: subscription.Name + "/" + fingerprint, Steps: steps}, nil
}

// SetAccess updates the system account and generated authorized_keys to match
// the selected SSH access mode. A password is returned only once, for modes
// that enable password authentication.
func (service SSHService) SetAccess(ctx context.Context, subscriptionName, access string) (string, int64, error) {
	operation, password, err := service.PrepareSetAccess(ctx, subscriptionName, access)
	if err != nil {
		return "", 0, err
	}
	operationID, err := service.Executor.Run(ctx, operation)
	if err != nil {
		return "", operationID, err
	}
	return password, operationID, nil
}

// PrepareSetAccess creates a journaled plan. Password changes are deliberately
// last because they cannot be safely rolled back without retaining a secret.
func (service SSHService) PrepareSetAccess(ctx context.Context, subscriptionName, access string) (plan.Plan, string, error) {
	if err := domain.ValidateSSHAccess(access); err != nil {
		return plan.Plan{}, "", err
	}
	if service.Users == nil {
		return plan.Plan{}, "", fmt.Errorf("user manager is required")
	}
	subscription, keys, err := service.subscriptionKeys(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, "", err
	}
	usesKeys := access == "key" || access == "key+password"
	usesPassword := access == "password" || access == "key+password"
	if usesKeys && len(keys) == 0 {
		return plan.Plan{}, "", fmt.Errorf("SSH access %q requires at least one SSH key", access)
	}
	password := ""
	if usesPassword {
		password, err = GenerateSSHPassword(24)
		if err != nil {
			return plan.Plan{}, "", err
		}
	}
	desiredKeys := keys
	if !usesKeys {
		desiredKeys = nil
	}
	previousShell := meta.LoginShell
	if subscription.SSHAccess == "none" {
		previousShell = meta.NoLoginShell
	}
	desiredShell := meta.LoginShell
	if access == "none" {
		desiredShell = meta.NoLoginShell
	}
	var undoFile func(context.Context) error
	steps := []plan.Step{{Name: "write generated authorized_keys", Preview: "write " + authorizedKeysPath(subscription), Do: func(ctx context.Context) error {
		var writeErr error
		undoFile, writeErr = service.writeAuthorizedKeys(subscription, desiredKeys)
		return writeErr
	}, Undo: func(ctx context.Context) error {
		if undoFile == nil {
			return nil
		}
		return undoFile(ctx)
	}}, {Name: "set subscription shell", Preview: "/usr/sbin/usermod --shell " + desiredShell + " " + subscription.UnixUser, Do: func(ctx context.Context) error {
		return service.Users.SetShell(ctx, subscription.UnixUser, desiredShell)
	}, Undo: func(ctx context.Context) error {
		return service.Users.SetShell(ctx, subscription.UnixUser, previousShell)
	}}, {Name: "record SSH access in SQLite", Preview: "update SSH access to " + access, Do: func(ctx context.Context) error {
		return service.Store.UpdateSSHAccess(ctx, subscription.ID, access)
	}, Undo: func(ctx context.Context) error {
		return service.Store.UpdateSSHAccess(ctx, subscription.ID, subscription.SSHAccess)
	}}}
	if usesPassword {
		steps = append(steps, plan.Step{Name: "set subscription password", Preview: "/usr/sbin/chpasswd", Do: func(ctx context.Context) error {
			return service.Users.SetPassword(ctx, subscription.UnixUser, password)
		}})
	} else {
		steps = append(steps, plan.Step{Name: "lock subscription password", Preview: "/usr/sbin/usermod --lock " + subscription.UnixUser, Do: func(ctx context.Context) error {
			return service.Users.LockPassword(ctx, subscription.UnixUser)
		}})
	}
	return plan.Plan{Action: "ssh.access.set", Target: subscription.Name, Steps: steps}, password, nil
}

func (service SSHService) subscriptionKeys(ctx context.Context, subscriptionName string) (domain.Subscription, []domain.SSHKey, error) {
	if service.FS == nil || service.Commands == nil || service.Store == nil {
		return domain.Subscription{}, nil, fmt.Errorf("filesystem, commander, and SSH key store are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, nil, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, nil, err
	}
	if subscription.Status != "active" {
		return domain.Subscription{}, nil, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	keys, err := service.Store.ListSSHKeys(ctx, subscription.ID)
	if err != nil {
		return domain.Subscription{}, nil, fmt.Errorf("list SSH keys: %w", err)
	}
	return subscription, keys, nil
}

func (service SSHService) validatePublicKey(ctx context.Context, publicKey string) (fingerprint, comment, normalized string, err error) {
	normalized = strings.TrimSpace(publicKey)
	if normalized == "" || strings.Contains(normalized, "\n") {
		return "", "", "", fmt.Errorf("SSH public key must be exactly one non-empty line")
	}
	result, err := service.Commands.RunWithStdin(ctx, strings.NewReader(normalized+"\n"), "/usr/bin/ssh-keygen", "-l", "-f", "-")
	if err != nil {
		output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
		if output == "" {
			return "", "", "", fmt.Errorf("validate SSH public key: %w", err)
		}
		return "", "", "", fmt.Errorf("validate SSH public key: %w: %s", err, output)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "SHA256:") {
		return "", "", "", fmt.Errorf("parse SSH key fingerprint")
	}
	parts := strings.Fields(normalized)
	if len(parts) > 2 {
		comment = strings.Join(parts[2:], " ")
	}
	return fields[1], comment, normalized, nil
}

func authorizedKeysPath(subscription domain.Subscription) string {
	return filepath.Join(subscription.Home, ".ssh", "authorized_keys")
}

func renderAuthorizedKeys(keys []domain.SSHKey) []byte {
	var builder strings.Builder
	builder.WriteString("# GENERATED BY PROVCTL - DO NOT EDIT\n")
	for _, key := range keys {
		builder.WriteString(key.PublicKey)
		builder.WriteByte('\n')
	}
	return []byte(builder.String())
}

func (service SSHService) writeAuthorizedKeys(subscription domain.Subscription, keys []domain.SSHKey) (func(context.Context) error, error) {
	directory, path := filepath.Dir(authorizedKeysPath(subscription)), authorizedKeysPath(subscription)
	directoryExisted := true
	if _, err := service.FS.Stat(directory); os.IsNotExist(err) {
		directoryExisted = false
	} else if err != nil {
		return nil, fmt.Errorf("inspect SSH directory: %w", err)
	}
	previous, err := service.FS.ReadFile(path)
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		return nil, fmt.Errorf("read existing authorized_keys: %w", err)
	}
	if err := service.FS.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := service.FS.Chown(directory, subscription.UnixUID, subscription.UnixUID); err != nil {
		return nil, err
	}
	if err := service.FS.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	if err := service.FS.WriteFileAtomic(path, renderAuthorizedKeys(keys), 0o600); err != nil {
		return nil, err
	}
	if err := service.FS.Chown(path, subscription.UnixUID, subscription.UnixUID); err != nil {
		return nil, err
	}
	return func(context.Context) error {
		if missing {
			if err := service.FS.Remove(path); err != nil {
				return err
			}
			if !directoryExisted {
				return service.FS.Remove(directory)
			}
			return nil
		}
		if err := service.FS.WriteFileAtomic(path, previous, 0o600); err != nil {
			return err
		}
		return service.FS.Chown(path, subscription.UnixUID, subscription.UnixUID)
	}, nil
}
