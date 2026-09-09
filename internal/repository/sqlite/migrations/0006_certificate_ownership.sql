-- Existing certificates were issued by provctl before adoption existed.
-- Explicit ownership prevents an adopted legacy lineage from being deleted
-- merely because its Certbot name happens to use the provctl- prefix.
ALTER TABLE certificates ADD COLUMN managed INTEGER NOT NULL DEFAULT 1 CHECK (managed IN (0, 1));
