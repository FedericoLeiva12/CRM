-- Record deletion is opt-in. Existing write grants keep writing but lose implicit
-- permanent deletion until an administrator explicitly grants it.
ALTER TABLE permissions ADD COLUMN can_delete boolean NOT NULL DEFAULT false;
ALTER TABLE permissions ADD CONSTRAINT permissions_delete_requires_write CHECK (NOT can_delete OR can_write);
