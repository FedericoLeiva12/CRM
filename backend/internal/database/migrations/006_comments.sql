-- Comments are timeline entries with type 'comment' (see 004). They gain an
-- optional parent for one level of replies, an edit timestamp, and a tombstone
-- marker so a deleted comment that still has replies keeps the thread intact.
ALTER TABLE activities ADD COLUMN parent_id text REFERENCES activities(id) ON DELETE CASCADE;
ALTER TABLE activities ADD COLUMN edited_at timestamptz;
ALTER TABLE activities ADD COLUMN deleted_at timestamptz;
ALTER TABLE activities ADD CONSTRAINT activities_parent_not_self CHECK (parent_id IS NULL OR parent_id <> id);
CREATE INDEX activities_parent ON activities(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX activities_comments ON activities(section_id, record_id, created_at DESC, id DESC) WHERE type = 'comment';

-- Structured mentions. Each row is also the mentioned principal's notification:
-- read_at is null until they mark it read. principal_id is polymorphic (user or
-- agent), so the cleanup triggers below stand in for a foreign key.
CREATE TABLE comment_mentions(
  id text PRIMARY KEY,
  entry_id text NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('user', 'agent')),
  principal_id text NOT NULL,
  handle text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  read_at timestamptz,
  UNIQUE(entry_id, kind, principal_id)
);
CREATE INDEX comment_mentions_principal ON comment_mentions(kind, principal_id, created_at DESC, id DESC);
CREATE INDEX comment_mentions_unread ON comment_mentions(kind, principal_id) WHERE read_at IS NULL;

-- Stable @handles. Users and agents share one namespace so a handle never
-- resolves to two principals. Handles are lowercase, 1-32 characters from
-- [a-z0-9_-], and start with a letter or digit. The trigger assigns one on
-- insert, so every creation path gets a handle without application changes.
CREATE EXTENSION IF NOT EXISTS unaccent;
ALTER TABLE users ADD COLUMN handle text;
ALTER TABLE agents ADD COLUMN handle text;

CREATE FUNCTION sira_unique_handle(source text, fallback text, own_id text) RETURNS text AS $$
DECLARE
  base text;
  candidate text;
  attempt integer := 1;
BEGIN
  -- Serializes assignment so concurrent inserts cannot pick the same handle.
  PERFORM pg_advisory_xact_lock(73142003);
  base := btrim(left(btrim(regexp_replace(lower(unaccent(coalesce(source, ''))), '[^a-z0-9]+', '-', 'g'), '-'), 28), '-');
  IF base = '' THEN
    base := fallback;
  END IF;
  candidate := base;
  LOOP
    EXIT WHEN NOT EXISTS (SELECT 1 FROM users WHERE handle = candidate AND id <> own_id)
      AND NOT EXISTS (SELECT 1 FROM agents WHERE handle = candidate AND id <> own_id);
    attempt := attempt + 1;
    candidate := btrim(left(base, 31 - length(attempt::text)), '-') || '-' || attempt;
  END LOOP;
  RETURN candidate;
END
$$ LANGUAGE plpgsql;

CREATE FUNCTION sira_assign_handle() RETURNS trigger AS $$
BEGIN
  IF NEW.handle IS NULL OR NEW.handle = '' THEN
    IF TG_TABLE_NAME = 'users' THEN
      NEW.handle := sira_unique_handle(coalesce(nullif(btrim(NEW.name), ''), split_part(NEW.email, '@', 1)), 'member', NEW.id);
    ELSE
      NEW.handle := sira_unique_handle(NEW.name, 'agent', NEW.id);
    END IF;
  ELSE
    -- The per-table unique indexes cannot see the other table, so an explicit handle is checked here.
    PERFORM pg_advisory_xact_lock(73142003);
    IF EXISTS (SELECT 1 FROM users WHERE handle = NEW.handle AND id <> NEW.id)
      OR EXISTS (SELECT 1 FROM agents WHERE handle = NEW.handle AND id <> NEW.id) THEN
      RAISE EXCEPTION 'handle % is already taken', NEW.handle USING ERRCODE = 'unique_violation';
    END IF;
  END IF;
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

DO $$
DECLARE
  item record;
BEGIN
  FOR item IN SELECT id, name, email FROM users ORDER BY created_at, id LOOP
    UPDATE users SET handle = sira_unique_handle(coalesce(nullif(btrim(item.name), ''), split_part(item.email, '@', 1)), 'member', item.id) WHERE id = item.id;
  END LOOP;
  FOR item IN SELECT id, name FROM agents ORDER BY created_at, id LOOP
    UPDATE agents SET handle = sira_unique_handle(item.name, 'agent', item.id) WHERE id = item.id;
  END LOOP;
END
$$;

ALTER TABLE users ALTER COLUMN handle SET NOT NULL;
ALTER TABLE agents ALTER COLUMN handle SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_handle_format CHECK (handle ~ '^[a-z0-9][a-z0-9_-]{0,31}$');
ALTER TABLE agents ADD CONSTRAINT agents_handle_format CHECK (handle ~ '^[a-z0-9][a-z0-9_-]{0,31}$');
CREATE UNIQUE INDEX users_handle_key ON users(handle);
CREATE UNIQUE INDEX agents_handle_key ON agents(handle);

CREATE TRIGGER users_assign_handle BEFORE INSERT OR UPDATE OF handle ON users FOR EACH ROW EXECUTE FUNCTION sira_assign_handle();
CREATE TRIGGER agents_assign_handle BEFORE INSERT OR UPDATE OF handle ON agents FOR EACH ROW EXECUTE FUNCTION sira_assign_handle();

CREATE FUNCTION sira_drop_mentions() RETURNS trigger AS $$
BEGIN
  DELETE FROM comment_mentions WHERE kind = TG_ARGV[0] AND principal_id = OLD.id;
  RETURN OLD;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_drop_mentions AFTER DELETE ON users FOR EACH ROW EXECUTE FUNCTION sira_drop_mentions('user');
CREATE TRIGGER agents_drop_mentions AFTER DELETE ON agents FOR EACH ROW EXECUTE FUNCTION sira_drop_mentions('agent');
