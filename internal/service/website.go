package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type WebsiteStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	DomainExists(context.Context, string) (bool, error)
	CreateWebsite(context.Context, domain.Website) (int64, error)
	DeleteWebsite(context.Context, int64) error
	CertificateByWebsite(context.Context, int64) (domain.Certificate, error)
	DeleteCertificateByWebsite(context.Context, int64) error
	SetWebsiteEnabled(context.Context, int64, bool) error
	SetWebsiteDocumentRoot(context.Context, int64, string) error
	SetWebsiteLogDirectory(context.Context, int64, string) error
	SetWebsiteTarget(context.Context, int64, string, int) error
	SetWebsiteSSL(context.Context, int64, bool, bool) error
	AddWebsiteAlias(context.Context, int64, string) error
	RemoveWebsiteAlias(context.Context, int64, string) error
	ListWebsites(context.Context, int64) ([]domain.Website, error)
}

// List returns websites of a subscription for read-only callers such as the TUI.
func (service WebsiteService) List(ctx context.Context, subscriptionID int64) ([]domain.Website, error) {
	websites, err := service.Store.ListWebsites(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list websites: %w", err)
	}
	return websites, nil
}

// ListForSubscription returns websites identified by their subscription name.
func (service WebsiteService) ListForSubscription(ctx context.Context, subscriptionName string) ([]domain.Website, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return nil, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	return service.List(ctx, subscription.ID)
}

// ReadLogs returns the final lines of one website access or error log.
func (service WebsiteService) ReadLogs(ctx context.Context, subscriptionName, primaryDomain string, errorLog bool, lines int) (string, error) {
	if service.FS == nil {
		return "", fmt.Errorf("filesystem is required")
	}
	if lines < 1 || lines > 1000 {
		return "", fmt.Errorf("log lines must be between 1 and 1000")
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return "", err
	}
	var website domain.Website
	found := false
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website = candidate
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	name := "access.log"
	if errorLog {
		name = "error.log"
	}
	contents, err := service.FS.ReadFile(filepath.Join(service.websiteLogDirectory(subscriptionName, website), name))
	if err != nil {
		return "", fmt.Errorf("read website log: %w", err)
	}
	entries := strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	if len(entries) > lines {
		entries = entries[len(entries)-lines:]
	}
	return strings.Join(entries, "\n") + "\n", nil
}

type ApacheVHostApplier interface {
	Apply(context.Context, string, []byte) (func(context.Context) error, error)
	ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error)
	SetVHostEnabled(context.Context, string, string, bool) (func(context.Context) error, error)
	RemoveVHost(context.Context, string, string) (func(context.Context) error, error)
}

// WebsiteService creates an isolated PHP-FPM website and its Apache vhost.
type WebsiteService struct {
	FS           system.FS
	Store        WebsiteStore
	Executor     plan.Executor
	Apache       ApacheVHostApplier
	PHPFPM       PHPFPMPoolApplier
	Version      PHPFPMVersion
	Config       config.Config
	Certificates CertificateRemover
}

// WebsiteRuntime owns the database connection used by a website command.
type WebsiteRuntime struct {
	Service    WebsiteService
	repository *sqlite.Repository
}

func NewProductionWebsiteRuntime(ctx context.Context, cfg config.Config) (*WebsiteRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, systemd)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &WebsiteRuntime{
		Service: WebsiteService{
			FS:           system.OSFS{},
			Store:        repository,
			Executor:     productionExecutor(repository),
			Apache:       Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service},
			PHPFPM:       PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd},
			Version:      version,
			Config:       cfg,
			Certificates: CertbotCertificateRemover{Commands: commander},
		},
		repository: repository,
	}, nil
}

func NewReadOnlyWebsiteRuntime(ctx context.Context, cfg config.Config) (*WebsiteRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, systemd)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &WebsiteRuntime{Service: WebsiteService{FS: system.OSFS{}, Store: repository, PHPFPM: PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd}, Version: version, Config: cfg}, repository: repository}, nil
}

func (runtime *WebsiteRuntime) Close() error { return runtime.repository.Close() }

func selectPHPFPM(ctx context.Context, cfg config.Config, fs system.FS, systemd system.Systemd) (PHPFPMVersion, error) {
	versions, err := DiscoverPHPFPM(ctx, fs, systemd)
	if err != nil {
		return PHPFPMVersion{}, err
	}
	return SelectPHPFPM(cfg.PHP.DefaultVersion, versions)
}

func (service WebsiteService) CreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareCreatePHPFPM(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// CreateStatic provisions an isolated static website and its Apache vhost.
func (service WebsiteService) CreateStatic(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareCreateStatic(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) AddAlias(ctx context.Context, subscriptionName, primaryDomain, alias string) (int64, error) {
	operation, err := service.PrepareAlias(ctx, subscriptionName, primaryDomain, alias, true)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) RemoveAlias(ctx context.Context, subscriptionName, primaryDomain, alias string) (int64, error) {
	operation, err := service.PrepareAlias(ctx, subscriptionName, primaryDomain, alias, false)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareAlias(ctx context.Context, subscriptionName, primaryDomain, alias string, add bool) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateDomain(alias); err != nil {
		return plan.Plan{}, err
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var website domain.Website
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website = candidate
			break
		}
	}
	if website.ID == 0 {
		return plan.Plan{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	aliases := append([]string(nil), website.Aliases...)
	if add {
		exists, err := service.Store.DomainExists(ctx, alias)
		if err != nil {
			return plan.Plan{}, err
		}
		if exists {
			return plan.Plan{}, fmt.Errorf("domain %q is already assigned", alias)
		}
		aliases = append(aliases, alias)
		sort.Strings(aliases)
	} else {
		filtered := aliases[:0]
		for _, value := range aliases {
			if value != alias {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == len(aliases) {
			return plan.Plan{}, fmt.Errorf("website alias %q not found", alias)
		}
		aliases = filtered
	}
	website.Aliases = aliases
	contents, err := service.RenderVHost(subscriptionName, website)
	if err != nil {
		return plan.Plan{}, err
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	var undoApache func(context.Context) error
	verb := map[bool]string{true: "add", false: "remove"}[add]
	steps := []plan.Step{{Name: "update Apache vhost aliases", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.Apply(ctx, vhostPath, contents)
		return err
	}, Undo: func(ctx context.Context) error { return undoApache(ctx) }}, {Name: verb + " website alias in SQLite", Preview: verb + " " + alias, Do: func(ctx context.Context) error {
		if add {
			return service.Store.AddWebsiteAlias(ctx, website.ID, alias)
		}
		return service.Store.RemoveWebsiteAlias(ctx, website.ID, alias)
	}, Undo: func(ctx context.Context) error {
		if add {
			return service.Store.RemoveWebsiteAlias(ctx, website.ID, alias)
		}
		return service.Store.AddWebsiteAlias(ctx, website.ID, alias)
	}}}
	return plan.Plan{Action: "website.alias." + verb, Target: subscriptionName + "/" + primaryDomain + "/" + alias, Steps: steps}, nil
}

// RenderVHost renders the complete HTTP and, when enabled, HTTPS vhost for a
// persisted website. It is shared by lifecycle services so generated config
// stays consistent across website and certificate operations.
func (service WebsiteService) RenderVHost(subscriptionName string, website domain.Website) ([]byte, error) {
	logDir := service.websiteLogDirectory(subscriptionName, website)
	var (
		httpContents []byte
		err          error
	)
	switch website.Type {
	case domain.WebsiteStatic:
		httpContents, err = render.RenderApacheStaticHTTP(render.ApacheStaticVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, DocumentRoot: website.DocumentRoot, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir, ForceHTTPS: website.ForceHTTPS})
	case domain.WebsiteProxy:
		httpContents, err = render.RenderApacheProxyHTTP(render.ApacheProxyVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, Target: website.Target, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir, ForceHTTPS: website.ForceHTTPS}, service.Config.Apache.AllowedProxyHosts)
	case domain.WebsiteRedirect:
		httpContents, err = render.RenderApacheRedirectHTTP(render.ApacheRedirectVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, Target: website.Target, RedirectCode: website.RedirectCode, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir, ForceHTTPS: website.ForceHTTPS})
	case domain.WebsitePHPFPM:
		httpContents, err = render.RenderApachePHPFPMHTTP(render.ApacheHTTPVHost{Subscription: subscriptionName, PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, DocumentRoot: website.DocumentRoot, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, FPMSocket: phpSocket(subscriptionName, website.PrimaryDomain), ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir, ForceHTTPS: website.ForceHTTPS})
	default:
		return nil, fmt.Errorf("unsupported website type %q", website.Type)
	}
	if err != nil || !website.SSLEnabled {
		return httpContents, err
	}
	tlsContents, err := service.renderTLSVHost(subscriptionName, website, logDir)
	if err != nil {
		return nil, err
	}
	return append(append(httpContents, '\n'), tlsContents...), nil
}

func (service WebsiteService) websiteLogDirectory(subscriptionName string, website domain.Website) string {
	if website.LogDirectory != "" {
		return website.LogDirectory
	}
	return filepath.Join(meta.LogDir, subscriptionName, website.PrimaryDomain)
}

func (service WebsiteService) renderTLSVHost(subscriptionName string, website domain.Website, logDir string) ([]byte, error) {
	lineage := website.CertificateName
	if lineage == "" {
		// Legacy rows created before schema v4 retain the historical lineage
		// until their certificate metadata has been reconciled.
		lineage = meta.FilePrefix + subscriptionName + "-" + website.PrimaryDomain
	}
	lineageDir := filepath.Join(meta.LetsEncryptLiveDir, lineage)
	certificateFile := filepath.Join(lineageDir, "fullchain.pem")
	certificateKey := filepath.Join(lineageDir, "privkey.pem")
	switch website.Type {
	case domain.WebsiteStatic:
		return render.RenderApacheStaticTLS(render.ApacheStaticTLSVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, DocumentRoot: website.DocumentRoot, CertificateFile: certificateFile, CertificateKey: certificateKey, LogDir: logDir})
	case domain.WebsiteProxy:
		return render.RenderApacheProxyTLS(render.ApacheProxyTLSVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, Target: website.Target, CertificateFile: certificateFile, CertificateKey: certificateKey, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir}, service.Config.Apache.AllowedProxyHosts)
	case domain.WebsiteRedirect:
		return render.RenderApacheRedirectTLS(render.ApacheRedirectTLSVHost{PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, Target: website.Target, RedirectCode: website.RedirectCode, CertificateFile: certificateFile, CertificateKey: certificateKey, LogDir: logDir})
	case domain.WebsitePHPFPM:
		return render.RenderApachePHPFPMTLS(render.ApacheTLSVHost{Subscription: subscriptionName, PrimaryDomain: website.PrimaryDomain, Aliases: website.Aliases, DocumentRoot: website.DocumentRoot, CertificateFile: certificateFile, CertificateKey: certificateKey, FPMSocket: phpSocket(subscriptionName, website.PrimaryDomain), ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir})
	default:
		return nil, fmt.Errorf("unsupported website type %q", website.Type)
	}
}

// SetEnabled changes one website's Apache vhost and persisted enabled state.
func (service WebsiteService) SetEnabled(ctx context.Context, subscriptionName, primaryDomain string, enabled bool) (int64, error) {
	operation, err := service.PrepareSetEnabled(ctx, subscriptionName, primaryDomain, enabled)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// SetDocumentRoot changes the rendered root of a static or PHP-FPM website.
// It never moves data: the selected directory must already exist inside the
// subscription home after symlinks are resolved.
func (service WebsiteService) SetDocumentRoot(ctx context.Context, subscriptionName, primaryDomain, documentRoot string) (int64, error) {
	operation, err := service.PrepareSetDocumentRoot(ctx, subscriptionName, primaryDomain, documentRoot)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// SetLogDirectory changes the Apache access/error-log parent for one domain.
// Overrides stay within the subscription's protected provctl log root.
func (service WebsiteService) SetLogDirectory(ctx context.Context, subscriptionName, primaryDomain, logDirectory string) (int64, error) {
	operation, err := service.PrepareSetLogDirectory(ctx, subscriptionName, primaryDomain, logDirectory)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareSetLogDirectory(ctx context.Context, subscriptionName, primaryDomain, logDirectory string) (plan.Plan, error) {
	if service.FS == nil || service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("filesystem and Apache vhost applier are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	website, err := service.websiteByDomain(ctx, subscription, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	resolved, err := validateWebsiteLogDirectory(subscriptionName, logDirectory)
	if err != nil {
		return plan.Plan{}, err
	}
	if website.LogDirectory == resolved {
		return plan.Plan{}, fmt.Errorf("website %q already uses log directory %q", primaryDomain, resolved)
	}
	updated := website
	updated.LogDirectory = resolved
	contents, err := service.RenderVHost(subscriptionName, updated)
	if err != nil {
		return plan.Plan{}, err
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: "create website log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", resolved, subscription.UnixUID, resolved), Do: service.createOwnedDirectory(resolved, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return nil }}, {Name: "update Apache log directory", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var applyErr error
		undoApache, applyErr = service.Apache.Apply(ctx, vhostPath, contents)
		return applyErr
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, {Name: "record website log directory", Preview: "set log directory in SQLite", Do: func(ctx context.Context) error {
		return service.Store.SetWebsiteLogDirectory(ctx, website.ID, resolved)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetWebsiteLogDirectory(ctx, website.ID, website.LogDirectory)
	}}}
	return plan.Plan{Action: "website.set-log-directory", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

func validateWebsiteLogDirectory(subscriptionName, logDirectory string) (string, error) {
	if logDirectory == "" || !filepath.IsAbs(logDirectory) {
		return "", fmt.Errorf("log directory must be an absolute path")
	}
	clean := filepath.Clean(logDirectory)
	root := filepath.Join(meta.LogDir, subscriptionName)
	relative, err := filepath.Rel(root, clean)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("log directory %q must be inside %q", logDirectory, root)
	}
	return clean, nil
}

// SetTarget changes a proxy upstream or redirect destination without changing
// its domain identity, aliases, or TLS lineage.
func (service WebsiteService) SetTarget(ctx context.Context, subscriptionName, primaryDomain, target string, redirectCode int) (int64, error) {
	operation, err := service.PrepareSetTarget(ctx, subscriptionName, primaryDomain, target, redirectCode)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareSetTarget(ctx context.Context, subscriptionName, primaryDomain, target string, redirectCode int) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	website, err := service.websiteByDomain(ctx, subscription, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	if website.Type != domain.WebsiteProxy && website.Type != domain.WebsiteRedirect {
		return plan.Plan{}, fmt.Errorf("website type %q does not have a target", website.Type)
	}
	updated := website
	updated.Target = target
	if website.Type == domain.WebsiteRedirect {
		updated.RedirectCode = redirectCode
	}
	if updated.Target == website.Target && updated.RedirectCode == website.RedirectCode {
		return plan.Plan{}, fmt.Errorf("website %q target is unchanged", primaryDomain)
	}
	contents, err := service.RenderVHost(subscriptionName, updated)
	if err != nil {
		return plan.Plan{}, err
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: "update Apache website target", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var applyErr error
		undoApache, applyErr = service.Apache.Apply(ctx, vhostPath, contents)
		return applyErr
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, {Name: "record website target", Preview: "set target in SQLite", Do: func(ctx context.Context) error {
		return service.Store.SetWebsiteTarget(ctx, website.ID, updated.Target, updated.RedirectCode)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetWebsiteTarget(ctx, website.ID, website.Target, website.RedirectCode)
	}}}
	return plan.Plan{Action: "website.set-target", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

// PrepareSetDocumentRoot validates a root, renders a replacement vhost and
// records the root only after Apache has accepted the replacement.
func (service WebsiteService) PrepareSetDocumentRoot(ctx context.Context, subscriptionName, primaryDomain, documentRoot string) (plan.Plan, error) {
	if service.FS == nil || service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("filesystem and Apache vhost applier are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	website, err := service.websiteByDomain(ctx, subscription, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	if website.Type != domain.WebsiteStatic && website.Type != domain.WebsitePHPFPM {
		return plan.Plan{}, fmt.Errorf("website type %q does not have a document root", website.Type)
	}
	resolvedRoot, err := validateWebsiteDocumentRoot(service.FS, subscription.Home, documentRoot)
	if err != nil {
		return plan.Plan{}, err
	}
	if filepath.Clean(website.DocumentRoot) == resolvedRoot {
		return plan.Plan{}, fmt.Errorf("website %q already uses document root %q", primaryDomain, resolvedRoot)
	}
	updated := website
	updated.DocumentRoot = resolvedRoot
	contents, err := service.RenderVHost(subscriptionName, updated)
	if err != nil {
		return plan.Plan{}, err
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: "update Apache document root", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var applyErr error
		undoApache, applyErr = service.Apache.Apply(ctx, vhostPath, contents)
		return applyErr
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, {Name: "record website document root", Preview: "set document root in SQLite", Do: func(ctx context.Context) error {
		return service.Store.SetWebsiteDocumentRoot(ctx, website.ID, resolvedRoot)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetWebsiteDocumentRoot(ctx, website.ID, website.DocumentRoot)
	}}}
	return plan.Plan{Action: "website.set-document-root", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

func (service WebsiteService) websiteByDomain(ctx context.Context, subscription domain.Subscription, primaryDomain string) (domain.Website, error) {
	websites, err := service.List(ctx, subscription.ID)
	if err != nil {
		return domain.Website{}, err
	}
	for _, website := range websites {
		if website.PrimaryDomain == primaryDomain {
			return website, nil
		}
	}
	return domain.Website{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscription.Name)
}

func validateWebsiteDocumentRoot(fs system.FS, home, documentRoot string) (string, error) {
	if documentRoot == "" || !filepath.IsAbs(documentRoot) {
		return "", fmt.Errorf("document root must be an absolute path")
	}
	for _, component := range strings.FieldsFunc(documentRoot, func(r rune) bool { return r == '/' || r == '\\' }) {
		if component == ".." {
			return "", fmt.Errorf("document root must not contain parent traversal")
		}
	}
	resolvedHome, err := fs.EvalSymlinks(filepath.Clean(home))
	if err != nil {
		return "", fmt.Errorf("resolve subscription home: %w", err)
	}
	resolvedRoot, err := fs.EvalSymlinks(filepath.Clean(documentRoot))
	if err != nil {
		return "", fmt.Errorf("resolve document root: %w", err)
	}
	info, err := fs.Stat(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("inspect document root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("document root %q is not a directory", documentRoot)
	}
	relative, err := filepath.Rel(resolvedHome, resolvedRoot)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("document root %q must be inside subscription home", documentRoot)
	}
	return resolvedRoot, nil
}

// Delete removes generated Apache artifacts and the website record. Site data
// and logs are intentionally retained for administrator-managed recovery.
func (service WebsiteService) Delete(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareDelete(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareDelete(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var website domain.Website
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website = candidate
			break
		}
	}
	if website.ID == 0 {
		return plan.Plan{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	certificate, certificateErr := service.Store.CertificateByWebsite(ctx, website.ID)
	hasCertificate := certificateErr == nil
	if certificateErr != nil && !strings.Contains(certificateErr.Error(), "not found") {
		return plan.Plan{}, certificateErr
	}
	if hasCertificate && certificate.Managed && service.Certificates == nil {
		return plan.Plan{}, fmt.Errorf("certificate remover is required to delete TLS website %q", primaryDomain)
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: "remove Apache vhost", Preview: "remove " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.RemoveVHost(ctx, vhostPath, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error { return undoApache(ctx) }}}
	if website.Type == domain.WebsitePHPFPM && website.PHPVersion != "" && service.PHPFPM != nil {
		version := PHPFPMVersion{Version: website.PHPVersion, Binary: filepath.Join("/usr/sbin", "php-fpm"+website.PHPVersion), Service: "php" + website.PHPVersion + "-fpm.service"}
		poolPath := phpPoolPath(version, subscriptionName, website.PrimaryDomain)
		steps = append(steps, plan.Step{Name: "remove domain PHP-FPM pool", Preview: "remove " + poolPath, Do: func(ctx context.Context) error {
			_, err := service.PHPFPM.RemovePool(ctx, version, poolPath)
			return err
		}})
	}
	if hasCertificate {
		if certificate.Managed {
			steps = append(steps, plan.Step{Name: "delete Certbot certificate", Preview: "certbot delete --cert-name " + certificate.Lineage, Do: func(ctx context.Context) error {
				return service.Certificates.Delete(ctx, certificate.Lineage)
			}})
		}
		steps = append(steps, plan.Step{Name: "delete certificate metadata", Preview: "delete certificate metadata from SQLite", Do: func(ctx context.Context) error {
			return service.Store.DeleteCertificateByWebsite(ctx, website.ID)
		}})
	}
	steps = append(steps, plan.Step{Name: "remove website from SQLite", Preview: "delete website and domains from SQLite; retain site data and logs", Do: func(ctx context.Context) error {
		return service.Store.DeleteWebsite(ctx, website.ID)
	}, Undo: func(ctx context.Context) error {
		_, err := service.Store.CreateWebsite(ctx, website)
		return err
	}})
	return plan.Plan{Action: "website.delete", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

// PrepareSetEnabled builds a reversible enable or disable operation.
func (service WebsiteService) PrepareSetEnabled(ctx context.Context, subscriptionName, primaryDomain string, enabled bool) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var website domain.Website
	found := false
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website, found = candidate, true
			break
		}
	}
	if !found {
		return plan.Plan{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	if website.Enabled == enabled {
		return plan.Plan{}, fmt.Errorf("website %q is already enabled=%t", primaryDomain, enabled)
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: map[bool]string{true: "enable Apache vhost", false: "disable Apache vhost"}[enabled], Preview: fmt.Sprintf("set %s enabled=%t", enabledPath, enabled), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.SetVHostEnabled(ctx, vhostPath, enabledPath, enabled)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, {Name: "record website enabled state", Preview: fmt.Sprintf("set website %d enabled=%t in SQLite", website.ID, enabled), Do: func(ctx context.Context) error {
		return service.Store.SetWebsiteEnabled(ctx, website.ID, enabled)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetWebsiteEnabled(ctx, website.ID, website.Enabled)
	}}}
	return plan.Plan{Action: "website.set-enabled", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

func (service WebsiteService) CreateProxy(ctx context.Context, subscriptionName, primaryDomain, target string) (int64, error) {
	operation, err := service.PrepareCreateProxy(ctx, subscriptionName, primaryDomain, target)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareCreateProxy(ctx context.Context, subscriptionName, primaryDomain, target string) (plan.Plan, error) {
	subscription, logDir, vhostPath, enabledPath, err := service.prepareHTTPWebsite(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	contents, err := render.RenderApacheProxyHTTP(render.ApacheProxyVHost{PrimaryDomain: primaryDomain, Target: target, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir}, service.Config.Apache.AllowedProxyHosts)
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteProxy, PrimaryDomain: primaryDomain, Target: target, Enabled: true}
	return service.createHTTPOnlyPlan(subscription, website, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) CreateRedirect(ctx context.Context, subscriptionName, primaryDomain, target string, code int) (int64, error) {
	operation, err := service.PrepareCreateRedirect(ctx, subscriptionName, primaryDomain, target, code)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareCreateRedirect(ctx context.Context, subscriptionName, primaryDomain, target string, code int) (plan.Plan, error) {
	subscription, logDir, vhostPath, enabledPath, err := service.prepareHTTPWebsite(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	contents, err := render.RenderApacheRedirectHTTP(render.ApacheRedirectVHost{PrimaryDomain: primaryDomain, Target: target, RedirectCode: code, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteRedirect, PrimaryDomain: primaryDomain, Target: target, RedirectCode: code, Enabled: true}
	return service.createHTTPOnlyPlan(subscription, website, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) prepareHTTPWebsite(ctx context.Context, subscriptionName, primaryDomain string) (domain.Subscription, string, string, string, error) {
	if service.Apache == nil {
		return domain.Subscription{}, "", "", "", fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	if subscription.Status != "active" {
		return domain.Subscription{}, "", "", "", fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	if err := service.ensureWebsiteQuota(ctx, subscription); err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return domain.Subscription{}, "", "", "", fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return domain.Subscription{}, "", "", "", fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	return subscription, logDir, vhostPath, filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath)), nil
}

func (service WebsiteService) ensureWebsiteQuota(ctx context.Context, subscription domain.Subscription) error {
	if subscription.QuotaWebsites == 0 {
		return nil
	}
	websites, err := service.Store.ListWebsites(ctx, subscription.ID)
	if err != nil {
		return fmt.Errorf("list websites: %w", err)
	}
	if len(websites) >= subscription.QuotaWebsites {
		return fmt.Errorf("subscription %q has reached its website quota of %d", subscription.Name, subscription.QuotaWebsites)
	}
	return nil
}

func (service WebsiteService) createHTTPOnlyPlan(subscription domain.Subscription, website domain.Website, logDir, vhostPath, enabledPath string, contents []byte) plan.Plan {
	steps := []plan.Step{{Name: "create website log directory", Preview: "create " + logDir, Do: service.createOwnedDirectory(logDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(logDir) }}}
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: "create " + path, Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

// PrepareCreateStatic validates state and prepares a static website without changes.
func (service WebsiteService) PrepareCreateStatic(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "active" {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	if err := service.ensureWebsiteQuota(ctx, subscription); err != nil {
		return plan.Plan{}, err
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	siteRoot := filepath.Join(subscription.Home, "sites", primaryDomain)
	documentRoot := filepath.Join(siteRoot, "public")
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	contents, err := render.RenderApacheStaticHTTP(render.ApacheStaticVHost{PrimaryDomain: primaryDomain, DocumentRoot: documentRoot, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteStatic, PrimaryDomain: primaryDomain, DocumentRoot: documentRoot, Enabled: true}
	return service.createStaticPlan(subscription, website, siteRoot, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) PrepareCreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.PHPFPM == nil || service.Version.Version == "" || service.Version.Binary == "" || service.Version.Service == "" {
		return plan.Plan{}, fmt.Errorf("PHP-FPM version and pool applier are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "active" {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	if err := service.ensureWebsiteQuota(ctx, subscription); err != nil {
		return plan.Plan{}, err
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	siteRoot := filepath.Join(subscription.Home, "sites", primaryDomain)
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	fpmLogDir := phpLogDir(subscription.Name, primaryDomain)
	fpmErrorLog := phpErrorLog(subscription.Name, primaryDomain)
	socket := phpSocket(subscription.Name, primaryDomain)
	poolPath := phpPoolPath(service.Version, subscription.Name, primaryDomain)
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	contents, err := render.RenderApachePHPFPMHTTP(render.ApacheHTTPVHost{Subscription: subscription.Name, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, FPMSocket: socket, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	poolContents, err := render.RenderPHPFPMPool(render.PHPFPMPool{Name: phpPoolName(subscription.Name, primaryDomain), User: subscription.UnixUser, Home: subscription.Home, Socket: socket, MaxChildren: subscription.PHPMaxChildren, MemoryLimit: subscription.PHPMemoryLimit, UploadMax: subscription.PHPUploadMax, MaxExecTime: subscription.PHPMaxExecTime, PhpErrorLog: fpmErrorLog})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsitePHPFPM, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), PHPVersion: service.Version.Version, Enabled: true}
	return service.createPHPFPMPlan(subscription, website, siteRoot, logDir, fpmLogDir, fpmErrorLog, poolPath, socket, poolContents, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) createPHPFPMPlan(subscription domain.Subscription, website domain.Website, siteRoot, logDir, fpmLogDir, fpmErrorLog, poolPath, socket string, poolContents []byte, vhostPath, enabledPath string, contents []byte) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{
		{"create website root", siteRoot, 0o751}, {"create public directory", filepath.Join(siteRoot, "public"), 0o755}, {"create application directory", filepath.Join(siteRoot, "app"), 0o750}, {"create storage directory", filepath.Join(siteRoot, "storage"), 0o750},
	}
	steps := make([]plan.Step, 0, len(directories)+7)
	for _, directory := range directories {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s; chown %d:%d %s", directory.mode, directory.path, subscription.UnixUID, subscription.UnixUID, directory.path), Do: service.createOwnedDirectory(directory.path, subscription.UnixUID, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "create PHP-FPM log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", fpmLogDir, subscription.UnixUID, fpmLogDir), Do: service.createOwnedDirectory(fpmLogDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(fpmLogDir) }})
	steps = append(steps, plan.Step{Name: "create PHP-FPM error log", Preview: fmt.Sprintf("create 0600 %d:%d %s", subscription.UnixUID, subscription.UnixUID, fpmErrorLog), Do: service.createPHPErrorLog(fpmErrorLog, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(fpmErrorLog) }})
	steps = append(steps, plan.Step{Name: "create website log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", logDir, subscription.UnixUID, logDir), Do: service.createOwnedDirectory(logDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(logDir) }})
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: fmt.Sprintf("create 0640 root:%d %s", subscription.UnixUID, path), Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoPool func(context.Context) error
	steps = append(steps, plan.Step{Name: "install PHP-FPM pool", Preview: fmt.Sprintf("write %s; validate %s -t; reload %s", poolPath, service.Version.Binary, service.Version.Service), Do: func(ctx context.Context) error {
		var err error
		undoPool, err = service.PHPFPM.ApplyPool(ctx, service.Version, poolPath, poolContents, socket)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoPool == nil {
			return nil
		}
		return undoPool(ctx)
	}})
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: fmt.Sprintf("write %s and enable %s", vhostPath, enabledPath), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

func (service WebsiteService) createStaticPlan(subscription domain.Subscription, website domain.Website, siteRoot, logDir, vhostPath, enabledPath string, contents []byte) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{{"create website root", siteRoot, 0o751}, {"create public directory", filepath.Join(siteRoot, "public"), 0o755}, {"create website log directory", logDir, 0o750}}
	steps := make([]plan.Step, 0, len(directories)+4)
	for _, directory := range directories {
		directory := directory
		uid, gid := subscription.UnixUID, subscription.UnixUID
		if directory.path == logDir {
			uid = 0
		}
		steps = append(steps, plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s", directory.mode, directory.path), Do: service.createOwnedDirectory(directory.path, uid, gid, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: "create " + path, Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: fmt.Sprintf("write %s and enable %s", vhostPath, enabledPath), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

func (service WebsiteService) createOwnedDirectory(path string, uid, gid int, mode os.FileMode) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.MkdirAll(path, mode); err != nil {
			return err
		}
		if err := service.FS.Chown(path, uid, gid); err != nil {
			return err
		}
		return service.FS.Chmod(path, mode)
	}
}

func (service WebsiteService) createLogFile(path string, gid int) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.WriteFileAtomic(path, nil, 0o640); err != nil {
			return err
		}
		if err := service.FS.Chown(path, 0, gid); err != nil {
			return err
		}
		return service.FS.Chmod(path, 0o640)
	}
}

func (service WebsiteService) createPHPErrorLog(path string, uid int) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.WriteFileAtomic(path, nil, 0o600); err != nil {
			return err
		}
		if err := service.FS.Chown(path, uid, uid); err != nil {
			return err
		}
		return service.FS.Chmod(path, 0o600)
	}
}
