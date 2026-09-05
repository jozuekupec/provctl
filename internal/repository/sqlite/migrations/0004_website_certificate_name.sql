ALTER TABLE websites ADD COLUMN certificate_name TEXT;

UPDATE websites
SET certificate_name = 'provctl-site-' || id
WHERE certificate_name IS NULL;

CREATE UNIQUE INDEX idx_websites_certificate_name
ON websites(certificate_name)
WHERE certificate_name IS NOT NULL;
