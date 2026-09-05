ALTER TABLE certificates ADD COLUMN website_id INTEGER REFERENCES websites(id) ON DELETE SET NULL;

UPDATE certificates
SET website_id = (
    SELECT domains.website_id
    FROM domains
    WHERE domains.name = certificates.primary_domain
      AND domains.is_primary = 1
)
WHERE website_id IS NULL;

CREATE UNIQUE INDEX idx_certificates_website_id
ON certificates(website_id)
WHERE website_id IS NOT NULL;
