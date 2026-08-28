CREATE TABLE subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    unix_user TEXT NOT NULL UNIQUE,
    unix_uid INTEGER,
    home TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'suspended', 'archived')),
    php_version TEXT,
    php_max_children INTEGER NOT NULL DEFAULT 10,
    php_memory_limit TEXT NOT NULL DEFAULT '256M',
    php_upload_max TEXT NOT NULL DEFAULT '64M',
    php_max_exec_time INTEGER NOT NULL DEFAULT 60,
    ssh_access TEXT NOT NULL DEFAULT 'none' CHECK (ssh_access IN ('none', 'key', 'password', 'key+password')),
    quota_disk_bytes INTEGER,
    quota_websites INTEGER,
    quota_databases INTEGER,
    quota_backups INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE websites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('static', 'php-fpm', 'proxy', 'redirect')),
    document_root TEXT,
    target TEXT,
    redirect_code INTEGER CHECK (redirect_code IN (301, 302)),
    php_version TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    ssl_enabled INTEGER NOT NULL DEFAULT 0,
    force_https INTEGER NOT NULL DEFAULT 0,
    hsts INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    name TEXT NOT NULL UNIQUE,
    unicode TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_domains_primary ON domains(website_id) WHERE is_primary = 1;

CREATE TABLE databases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    name TEXT NOT NULL UNIQUE,
    db_user TEXT NOT NULL,
    db_host TEXT NOT NULL DEFAULT 'localhost',
    charset TEXT NOT NULL DEFAULT 'utf8mb4',
    collation TEXT NOT NULL DEFAULT 'utf8mb4_unicode_ci',
    created_at TEXT NOT NULL
);

CREATE TABLE certificates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    lineage TEXT NOT NULL UNIQUE,
    primary_domain TEXT NOT NULL,
    sans TEXT NOT NULL,
    issuer TEXT,
    not_before TEXT,
    not_after TEXT,
    last_checked_at TEXT
);

CREATE TABLE cron_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    schedule TEXT NOT NULL,
    command TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    comment TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE ssh_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    comment TEXT,
    fingerprint TEXT NOT NULL,
    public_key TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    path TEXT NOT NULL UNIQUE,
    size_bytes INTEGER,
    status TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed')),
    started_at TEXT NOT NULL,
    finished_at TEXT,
    error TEXT
);

CREATE TABLE operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    target TEXT NOT NULL,
    actor TEXT,
    status TEXT NOT NULL CHECK (status IN ('running', 'done', 'failed', 'rolled_back', 'inconsistent')),
    plan_json TEXT NOT NULL,
    error TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT
);

CREATE TABLE meta_kv (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
