package service

import (
	"context"
	"crypto/rand"
	"crypto/tls"
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
	Name      string
	Domains   []string
	NotBefore time.Time
	NotAfter  time.Time
	Issuer    string
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
	FS              system.FS
	Commands        system.Commander
	Directory       string
	LiveDirectory   string
	BackupDirectory string
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
			now := time.Now()
			if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
				return nil, fmt.Errorf("certificate for lineage %q is outside its validity period", lineage)
			}
			chain, err := manager.FS.ReadFile(filepath.Join(liveDirectory, lineage, "fullchain.pem"))
			if err != nil {
				return nil, fmt.Errorf("read certificate chain for lineage %q: %w", lineage, err)
			}
			key, err := manager.FS.ReadFile(filepath.Join(liveDirectory, lineage, "privkey.pem"))
			if err != nil {
				return nil, fmt.Errorf("read private key for lineage %q: %w", lineage, err)
			}
			pair, err := tls.X509KeyPair(chain, key)
			if err != nil {
				return nil, fmt.Errorf("certificate and private key do not form a usable pair for lineage %q", lineage)
			}
			if len(pair.Certificate) == 0 || string(pair.Certificate[0]) != string(certificate.Raw) {
				return nil, fmt.Errorf("certificate and fullchain disagree for lineage %q", lineage)
			}
			lineages = append(lineages, RenewalLineage{Name: lineage, Domains: domains, NotBefore: certificate.NotBefore, NotAfter: certificate.NotAfter, Issuer: certificate.Issuer.String()})
		}
	}
	return lineages, nil
}

func (manager CertbotRenewals) Reconfigure(ctx context.Context, lineage RenewalLineage, webroot string) error {
	if err := domain.ValidateCertificateName(lineage.Name); err != nil {
		return err
	}
	args := []string{"reconfigure", "--cert-name", lineage.Name, "--authenticator", "webroot", "--webroot-path", webroot, "--non-interactive"}
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

// Snapshot preserves the Certbot-owned configuration for operation rollback.
func (manager CertbotRenewals) Snapshot(_ context.Context, lineage string) (func(context.Context) error, error) {
	if err := domain.ValidateCertificateName(lineage); err != nil {
		return nil, err
	}
	path := filepath.Join(manager.directory(), lineage+".conf")
	info, err := manager.FS.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect renewal configuration: %w", err)
	}
	contents, err := manager.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot renewal configuration: %w", err)
	}
	backupDirectory := manager.BackupDirectory
	if backupDirectory == "" {
		backupDirectory = meta.RenewalBackupDir
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("name renewal backup: %w", err)
	}
	backupDirectory = filepath.Join(backupDirectory, lineage, fmt.Sprintf("%s-%x", time.Now().UTC().Format("20060102T150405Z"), nonce))
	if err := manager.FS.MkdirAll(backupDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create renewal backup directory: %w", err)
	}
	if err := manager.FS.WriteFileAtomic(filepath.Join(backupDirectory, "renewal.conf"), contents, 0o600); err != nil {
		return nil, fmt.Errorf("persist renewal backup: %w", err)
	}
	metadata := []byte(fmt.Sprintf("source=%s\nmode=%04o\n", path, info.Mode().Perm()))
	if err := manager.FS.WriteFileAtomic(filepath.Join(backupDirectory, "restore.txt"), metadata, 0o600); err != nil {
		return nil, fmt.Errorf("persist renewal restore instructions: %w", err)
	}
	return func(context.Context) error { return manager.FS.WriteFileAtomic(path, contents, info.Mode().Perm()) }, nil
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
	AddWebsiteAlias(context.Context, int64, string) error
}

type adoptionCertificateStore interface {
	CreateCertificate(context.Context, domain.Certificate) (int64, error)
	DeleteCertificateByWebsite(context.Context, int64) error
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
	if len(renewals) > 1 {
		return plan.Plan{}, errors.New("multiple certificates cover the adoption domain; resolve the ambiguous lineage before adoption")
	}
	if len(renewals) == 1 {
		if _, ok := store.(adoptionCertificateStore); !ok {
			return plan.Plan{}, errors.New("TLS adoption requires certificate metadata storage")
		}
		for _, alias := range renewals[0].Domains {
			if alias == options.Domain {
				continue
			}
			if err := domain.ValidateDomain(alias); err != nil {
				return plan.Plan{}, fmt.Errorf("cannot adopt certificate alias: %w", err)
			}
			exists, err := store.DomainExists(ctx, alias)
			if err != nil {
				return plan.Plan{}, err
			}
			if exists {
				return plan.Plan{}, fmt.Errorf("certificate alias %q is already assigned", alias)
			}
		}
	}
	if service.Commands == nil {
		return plan.Plan{}, errors.New("adoption requires Apache configuration inspection")
	}
	result, err := service.Commands.Run(ctx, "/usr/sbin/apache2ctl", "-S")
	if err != nil {
		return plan.Plan{}, commandError("inspect existing Apache virtual hosts", result, err)
	}
	names := []string{options.Domain}
	if len(renewals) == 1 {
		names = append(names, renewals[0].Domains...)
	}
	if err := checkAdoptionVHostConflicts(result.Stdout+"\n"+result.Stderr, names); err != nil {
		return plan.Plan{}, err
	}
	return service.adoptPlan(store, renewalManager, subscription, options, source, siteRoot, documentRoot, renewals), nil
}

func checkAdoptionVHostConflicts(dump string, domains []string) error {
	for _, line := range strings.Split(dump, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.Contains(fields[0], ":") && strings.HasPrefix(fields[2], "(") {
			fields = append([]string{"namevhost"}, fields[1:]...)
		}
		for index, field := range fields {
			if field != "namevhost" && field != "alias" && !(field == "server" && index > 0 && fields[index-1] == "default") {
				continue
			}
			if index+1 >= len(fields) {
				continue
			}
			pattern := strings.ToLower(fields[index+1])
			for _, name := range domains {
				matched, err := filepath.Match(pattern, name)
				if err != nil {
					return fmt.Errorf("cannot safely inspect Apache hostname pattern %q: %w", pattern, err)
				}
				if matched {
					return fmt.Errorf("domain %q is already served by Apache (%s); disable the legacy vhost explicitly before adoption", name, strings.TrimSpace(line))
				}
			}
		}
	}
	return nil
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
	if len(renewals) == 1 {
		website.CertificateName = renewals[0].Name
		website.SSLEnabled = true
		for _, alias := range renewals[0].Domains {
			if alias != options.Domain && !containsDomain(website.Aliases, alias) {
				website.Aliases = append(website.Aliases, alias)
			}
		}
	}
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
		if website.SSLEnabled {
			vhostContents, err = websiteService.RenderVHost(subscription.Name, website)
			if err != nil {
				return err
			}
		}
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
		if err != nil {
			return err
		}
		for _, alias := range website.Aliases {
			if err := store.AddWebsiteAlias(ctx, id, alias); err != nil {
				return errors.Join(err, store.DeleteWebsite(ctx, id))
			}
		}
		return nil
	}, Undo: func(ctx context.Context) error { return store.DeleteWebsite(ctx, website.ID) }})
	if len(renewals) > 0 {
		steps = append(steps, plan.Step{Name: "commit adopted artifacts before certificate renewal", Preview: "retain adopted system artifacts and SQLite records if certificate renewal verification fails", Do: func(context.Context) error { return nil }, Commit: true})
	}
	for _, lineage := range renewals {
		lineage := lineage
		certificateStore := store.(adoptionCertificateStore)
		steps = append(steps, plan.Step{Name: "record adopted certificate", Preview: "retain Certbot lineage " + lineage.Name, Do: func(ctx context.Context) error {
			_, err := certificateStore.CreateCertificate(ctx, domain.Certificate{SubscriptionID: subscription.ID, WebsiteID: website.ID, Lineage: lineage.Name, PrimaryDomain: options.Domain, SANs: lineage.Domains, Issuer: lineage.Issuer, Managed: false, NotBefore: lineage.NotBefore, NotAfter: lineage.NotAfter, LastCheckedAt: time.Now().UTC()})
			return err
		}, Undo: func(ctx context.Context) error { return certificateStore.DeleteCertificateByWebsite(ctx, website.ID) }})
		var restoreRenewal func(context.Context) error
		steps = append(steps, plan.Step{Name: "reconfigure certificate renewal " + lineage.Name, Preview: "certbot reconfigure --cert-name " + lineage.Name, Do: func(ctx context.Context) error {
			if recovery, ok := renewalManager.(interface {
				Snapshot(context.Context, string) (func(context.Context) error, error)
			}); ok {
				var err error
				restoreRenewal, err = recovery.Snapshot(ctx, lineage.Name)
				if err != nil {
					return err
				}
			}
			if err := renewalManager.Reconfigure(ctx, lineage, service.Config.Paths.ACMEChallenge); err != nil {
				if restoreRenewal != nil {
					return errors.Join(err, restoreRenewal(context.Background()))
				}
				return err
			}
			return nil
		}, Undo: func(ctx context.Context) error {
			if restoreRenewal != nil {
				return restoreRenewal(ctx)
			}
			return nil
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
