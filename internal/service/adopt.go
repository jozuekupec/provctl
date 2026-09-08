package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/system"
)

// RenewalLineage is the live Certbot lineage which must be kept renewable
// after an adopted document root has moved.
type RenewalLineage struct {
	Name    string
	Domains []string
}

// RenewalManager keeps certificate renewal configuration outside SQLite,
// where Certbot remains the source of truth.
type RenewalManager interface {
	Find(context.Context, string) ([]RenewalLineage, error)
	Reconfigure(context.Context, RenewalLineage, string) error
	Verify(context.Context, string) error
}

// CertbotRenewals reads Certbot's renewal files and reconfigures matching
// lineages with explicit Certbot arguments.
type CertbotRenewals struct {
	FS            system.FS
	Commands      system.Commander
	Directory     string
	LiveDirectory string
}

func (manager CertbotRenewals) Find(_ context.Context, domainName string) ([]RenewalLineage, error) {
	entries, err := manager.FS.ReadDir(manager.directory())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read Certbot renewal directory: %w", err)
	}
	var lineages []RenewalLineage
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		lineage := strings.TrimSuffix(entry.Name(), ".conf")
		if err := domain.ValidateCertificateName(lineage); err != nil {
			return nil, err
		}
		liveDirectory := manager.LiveDirectory
		if liveDirectory == "" {
			liveDirectory = "/etc/letsencrypt/live"
		}
		contents, err := manager.FS.ReadFile(filepath.Join(liveDirectory, lineage, "cert.pem"))
		if err != nil {
			return nil, fmt.Errorf("read certificate for lineage %q: %w", lineage, err)
		}
		block, _ := pem.Decode(contents)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("invalid PEM certificate for lineage %q", lineage)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate for lineage %q: %w", lineage, err)
		}
		domains := certificate.DNSNames
		if containsDomain(domains, domainName) {
			lineages = append(lineages, RenewalLineage{Name: lineage, Domains: domains})
		}
	}
	return lineages, nil
}

func (manager CertbotRenewals) Reconfigure(ctx context.Context, lineage RenewalLineage, webroot string) error {
	args := []string{"certonly", "--webroot", "-w", webroot}
	for _, name := range lineage.Domains {
		args = append(args, "-d", name)
	}
	args = append(args, "--cert-name", lineage.Name, "--keep-until-expiring", "--non-interactive")
	result, err := manager.Commands.Run(ctx, "/usr/bin/certbot", args...)
	if err != nil {
		return commandError("reconfigure certificate renewal", result, err)
	}
	return nil
}

func (manager CertbotRenewals) Verify(ctx context.Context, lineage string) error {
	result, err := manager.Commands.Run(ctx, "/usr/bin/certbot", "renew", "--cert-name", lineage, "--dry-run")
	if err != nil {
		return commandError("verify certificate renewal", result, err)
	}
	return nil
}

func (manager CertbotRenewals) directory() string {
	if manager.Directory != "" {
		return manager.Directory
	}
	return "/etc/letsencrypt/renewal"
}

func containsDomain(domains []string, name string) bool {
	for _, domain := range domains {
		if domain == name {
			return true
		}
	}
	return false
}

type SubscriptionAdoptOptions struct {
	Source string
	Domain string
	Copy   bool
	Backup bool
}

type subscriptionAdoptStore interface {
	SubscriptionStore
	DomainExists(context.Context, string) (bool, error)
	CreateWebsite(context.Context, domain.Website) (int64, error)
}

// Adopt imports an existing document root as one PHP-FPM website.
func (service SubscriptionService) Adopt(ctx context.Context, name string, options SubscriptionAdoptOptions) (int64, error) {
	operation, err := service.PrepareAdopt(ctx, name, options)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// PrepareAdopt validates the legacy source and produces one journaled plan.
func (service SubscriptionService) PrepareAdopt(ctx context.Context, name string, options SubscriptionAdoptOptions) (plan.Plan, error) {
	if service.Apache == nil || service.PHPFPM == nil || service.Commands == nil {
		return plan.Plan{}, errors.New("adopt requires Apache, PHP-FPM, and commander")
	}
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(options.Domain); err != nil {
		return plan.Plan{}, err
	}
	if options.Source == "" {
		return plan.Plan{}, errors.New("adopt source is required")
	}
	source, err := service.FS.EvalSymlinks(filepath.Clean(options.Source))
	if err != nil {
		return plan.Plan{}, fmt.Errorf("resolve adopt source: %w", err)
	}
	info, err := service.FS.Stat(source)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("inspect adopt source: %w", err)
	}
	if !info.IsDir() {
		return plan.Plan{}, fmt.Errorf("adopt source %q is not a directory", source)
	}
	root, err := service.FS.EvalSymlinks(filepath.Clean(service.Config.Paths.VHosts))
	if err != nil {
		return plan.Plan{}, fmt.Errorf("resolve vhosts root: %w", err)
	}
	if pathWithin(root, source) {
		return plan.Plan{}, fmt.Errorf("adopt source %q is inside vhosts root", source)
	}
	subscription, err := service.prepareSubscription(ctx, name, SubscriptionCreateOptions{})
	if err != nil {
		return plan.Plan{}, err
	}
	store, ok := service.Store.(subscriptionAdoptStore)
	if !ok {
		return plan.Plan{}, errors.New("adopt requires a website-capable subscription store")
	}
	exists, err := store.DomainExists(ctx, options.Domain)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("domain %q is already assigned", options.Domain)
	}
	siteRoot := filepath.Join(subscription.Home, "sites", options.Domain)
	documentRoot := filepath.Join(siteRoot, "public")
	if _, err := service.FS.Stat(documentRoot); err == nil {
		return plan.Plan{}, fmt.Errorf("adopt target %q already exists", documentRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return plan.Plan{}, fmt.Errorf("inspect adopt target: %w", err)
	}
	renewalManager := service.Renewals
	if renewalManager == nil {
		renewalManager = CertbotRenewals{FS: service.FS, Commands: service.Commands}
	}
	renewals, err := renewalManager.Find(ctx, options.Domain)
	if err != nil {
		return plan.Plan{}, err
	}
	return service.adoptPlan(store, renewalManager, subscription, options, source, siteRoot, documentRoot, renewals), nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)))
}

func (service SubscriptionService) adoptPlan(store subscriptionAdoptStore, renewalManager RenewalManager, subscription domain.Subscription, options SubscriptionAdoptOptions, source, siteRoot, documentRoot string, renewals []RenewalLineage) plan.Plan {
	websiteService := WebsiteService{FS: service.FS, Apache: service.Apache, PHPFPM: service.PHPFPM, Version: PHPFPMVersion{Version: subscription.PHPVersion, Binary: filepath.Join("/usr/sbin", "php-fpm"+subscription.PHPVersion), Service: "php" + subscription.PHPVersion + "-fpm.service"}, Config: service.Config}
	logDir := filepath.Join(meta.LogDir, subscription.Name, options.Domain)
	fpmLogDir := filepath.Join(meta.LogDir, subscription.Name)
	fpmErrorLog := filepath.Join(fpmLogDir, "php-fpm-error.log")
	poolPath := filepath.Join("/etc/php", subscription.PHPVersion, "fpm", "pool.d", meta.FilePrefix+subscription.Name+".conf")
	socket := filepath.Join("/run/php", meta.FilePrefix+subscription.Name+".sock")
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+options.Domain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	poolContents, _ := render.RenderPHPFPMPool(render.PHPFPMPool{Name: subscription.Name, Home: subscription.Home, Socket: socket, MaxChildren: subscription.PHPMaxChildren, MemoryLimit: subscription.PHPMemoryLimit, UploadMax: subscription.PHPUploadMax, MaxExecTime: subscription.PHPMaxExecTime, PhpErrorLog: fpmErrorLog})
	vhostContents, _ := render.RenderApachePHPFPMHTTP(render.ApacheHTTPVHost{Subscription: subscription.Name, PrimaryDomain: options.Domain, DocumentRoot: documentRoot, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, FPMSocket: socket, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir})
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsitePHPFPM, PrimaryDomain: options.Domain, DocumentRoot: documentRoot, PHPVersion: subscription.PHPVersion, Enabled: true}
	steps := make([]plan.Step, 0, 18+len(renewals)*2)
	if options.Backup {
		archive := filepath.Join(service.Config.Paths.Backups, "adopt", subscription.Name, filepath.Base(source)+"-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".tar")
		steps = append(steps, plan.Step{Name: "archive legacy document root", Preview: "create backup " + archive, Do: func(ctx context.Context) error {
			if err := service.FS.MkdirAll(filepath.Dir(archive), 0o700); err != nil {
				return err
			}
			_, err := service.Commands.Run(ctx, "/usr/bin/tar", "--create", "--file", archive, "--directory", filepath.Dir(source), "--", filepath.Base(source))
			return err
		}, Undo: func(context.Context) error { return service.FS.Remove(archive) }})
	}
	steps = append(steps, plan.Step{Name: "create Unix user", Preview: fmt.Sprintf("create locked user %s with UID %d", subscription.UnixUser, subscription.UnixUID), Do: func(ctx context.Context) error {
		if err := service.Users.Create(ctx, system.CreateUserOptions{Name: subscription.UnixUser, UID: subscription.UnixUID, Home: subscription.Home, Shell: meta.NoLoginShell, UserGroup: true, NoCreateHome: true}); err != nil {
			return err
		}
		return service.Users.LockPassword(ctx, subscription.UnixUser)
	}, Undo: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }})
	for _, directory := range []struct {
		name, path string
		mode       os.FileMode
	}{
		{"create subscription home", subscription.Home, 0o751},
		{"create sites directory", filepath.Join(subscription.Home, "sites"), 0o751},
		{"create temporary directory", filepath.Join(subscription.Home, "tmp"), 0o700},
		{"create session directory", filepath.Join(subscription.Home, "tmp", "sessions"), 0o700},
		{"create private directory", filepath.Join(subscription.Home, "private"), 0o700},
		{"create SSH directory", filepath.Join(subscription.Home, ".ssh"), 0o700},
		{"create website root", siteRoot, 0o751},
		{"create application directory", filepath.Join(siteRoot, "app"), 0o750},
		{"create storage directory", filepath.Join(siteRoot, "storage"), 0o750},
	} {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Preview: "create " + directory.path, Do: service.createDirectory(directory.path, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	for _, directory := range []struct {
		name, path string
		uid        int
		mode       os.FileMode
	}{
		{"create PHP-FPM log directory", fpmLogDir, 0, 0o750},
		{"create website log directory", logDir, 0, 0o750},
	} {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Preview: "create " + directory.path, Do: websiteService.createOwnedDirectory(directory.path, directory.uid, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "create PHP-FPM error log", Preview: "create " + fpmErrorLog, Do: websiteService.createPHPErrorLog(fpmErrorLog, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(fpmErrorLog) }})
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: "create " + path, Do: websiteService.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoPool func(context.Context) error
	steps = append(steps, plan.Step{Name: "install PHP-FPM pool", Preview: "write " + poolPath, Do: func(ctx context.Context) error {
		var err error
		undoPool, err = service.PHPFPM.ApplyPool(ctx, websiteService.Version, poolPath, poolContents, socket)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoPool == nil {
			return nil
		}
		return undoPool(ctx)
	}})
	steps = append(steps, plan.Step{Name: map[bool]string{true: "copy legacy document root", false: "move legacy document root"}[options.Copy], Preview: documentRoot, Do: service.transferDocumentRoot(source, documentRoot, options.Copy), Undo: service.undoTransfer(source, documentRoot, options.Copy)})
	steps = append(steps, plan.Step{Name: "assign document root ownership", Preview: fmt.Sprintf("chown -R %d:%d %s", subscription.UnixUID, subscription.UnixUID, documentRoot), Do: func(ctx context.Context) error {
		_, err := service.Commands.Run(ctx, "/usr/bin/chown", "--recursive", "--", fmt.Sprintf("%d:%d", subscription.UnixUID, subscription.UnixUID), documentRoot)
		return err
	}})
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, vhostContents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record subscription", Preview: "insert subscription into SQLite", Do: func(ctx context.Context) error {
		if err := store.CreateSubscription(ctx, subscription); err != nil {
			return err
		}
		created, err := store.SubscriptionByName(ctx, subscription.Name)
		if err != nil {
			return err
		}
		subscription.ID, website.SubscriptionID = created.ID, created.ID
		return nil
	}, Undo: func(ctx context.Context) error { return store.DeleteSubscription(ctx, subscription.Name) }})
	steps = append(steps, plan.Step{Name: "record website", Preview: "insert website into SQLite", Do: func(ctx context.Context) error {
		id, err := store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return store.DeleteWebsite(ctx, website.ID) }})
	for _, lineage := range renewals {
		lineage := lineage
		steps = append(steps, plan.Step{Name: "reconfigure certificate renewal " + lineage.Name, Preview: "certbot certonly --cert-name " + lineage.Name, Do: func(ctx context.Context) error {
			return renewalManager.Reconfigure(ctx, lineage, service.Config.Paths.ACMEChallenge)
		}})
		steps = append(steps, plan.Step{Name: "verify certificate renewal " + lineage.Name, Preview: "certbot renew --cert-name " + lineage.Name + " --dry-run", Do: func(ctx context.Context) error { return renewalManager.Verify(ctx, lineage.Name) }})
	}
	return plan.Plan{Action: "subscription.adopt", Target: subscription.Name + "/" + options.Domain, Steps: steps}
}

func (service SubscriptionService) transferDocumentRoot(source, target string, copyData bool) func(context.Context) error {
	return func(ctx context.Context) error {
		if copyData {
			_, err := service.Commands.Run(ctx, "/usr/bin/cp", "--archive", "--", source, target)
			return err
		}
		mover, ok := service.FS.(system.FileMover)
		if !ok {
			return errors.New("filesystem does not support atomic rename for adopt; use --copy only after review")
		}
		if err := mover.Rename(source, target); err != nil {
			return fmt.Errorf("move legacy document root (use --copy for an intentional cross-filesystem copy): %w", err)
		}
		return nil
	}
}

func (service SubscriptionService) undoTransfer(source, target string, copyData bool) func(context.Context) error {
	return func(context.Context) error {
		if copyData {
			return service.FS.RemoveAll(target)
		}
		mover, ok := service.FS.(system.FileMover)
		if !ok {
			return errors.New("filesystem does not support atomic rollback for adopt")
		}
		return mover.Rename(target, source)
	}
}
