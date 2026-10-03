-- Existing writers keep their write grant and receive its newly required read
-- grant. Read-only and denied grants stay unchanged; deletion is still opt-in.
UPDATE permissions SET can_read=true WHERE can_write AND NOT can_read;
ALTER TABLE permissions ADD CONSTRAINT permissions_write_requires_read CHECK (NOT can_write OR can_read);
