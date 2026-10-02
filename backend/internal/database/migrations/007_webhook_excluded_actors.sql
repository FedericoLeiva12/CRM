-- Per-endpoint "exclude events caused by these actors". An event whose actor
-- (the user or agent that made the change) is listed for an endpoint is never
-- enqueued for that endpoint. webhook.test and the mentioned side of
-- comment.mentioned are not affected. Purely additive.
CREATE TABLE webhook_excluded_actors (
  endpoint_id text NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('user', 'agent')),
  actor_id text NOT NULL CHECK (char_length(actor_id) BETWEEN 1 AND 200),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (endpoint_id, kind, actor_id)
);
CREATE INDEX webhook_excluded_actors_actor ON webhook_excluded_actors (kind, actor_id);

-- Events withheld from an endpoint because their actor is excluded. These are
-- not deliveries, so they never count as failures.
ALTER TABLE webhook_endpoints ADD COLUMN skipped_events bigint NOT NULL DEFAULT 0 CHECK (skipped_events >= 0);

-- actor_id is polymorphic (user or agent), so these triggers stand in for a
-- foreign key: deleting a principal removes it from every exclusion list.
CREATE FUNCTION sira_drop_excluded_actors() RETURNS trigger AS $$
BEGIN
  DELETE FROM webhook_excluded_actors WHERE kind = TG_ARGV[0] AND actor_id = OLD.id;
  RETURN OLD;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_drop_excluded_actors AFTER DELETE ON users FOR EACH ROW EXECUTE FUNCTION sira_drop_excluded_actors('user');
CREATE TRIGGER agents_drop_excluded_actors AFTER DELETE ON agents FOR EACH ROW EXECUTE FUNCTION sira_drop_excluded_actors('agent');
