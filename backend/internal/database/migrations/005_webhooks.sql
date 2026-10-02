CREATE TABLE webhook_endpoints (
  id text PRIMARY KEY,
  url text NOT NULL CHECK (char_length(url) BETWEEN 1 AND 2000),
  description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 200),
  event_types text[] NOT NULL CHECK (cardinality(event_types) > 0),
  section_id text,
  enabled boolean NOT NULL DEFAULT true,
  auto_disabled boolean NOT NULL DEFAULT false,
  consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
  signing_secret text,
  custom_header_name text,
  custom_header_value text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT webhook_endpoints_auth_present CHECK (
    coalesce(signing_secret, '') <> ''
    OR (coalesce(custom_header_name, '') <> '' AND coalesce(custom_header_value, '') <> '')
  )
);

CREATE TABLE webhook_outbox (
  id text PRIMARY KEY,
  event_type text NOT NULL,
  section_id text,
  payload text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_outbox_created_at ON webhook_outbox (created_at);

CREATE TABLE webhook_deliveries (
  id bigserial PRIMARY KEY,
  outbox_id text NOT NULL REFERENCES webhook_outbox (id) ON DELETE CASCADE,
  endpoint_id text NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
  event_type text NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'inflight', 'succeeded', 'failed')),
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  claimed_at timestamptz,
  last_status_code integer,
  last_latency_ms integer,
  last_response text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (outbox_id, endpoint_id)
);
CREATE INDEX webhook_deliveries_pending ON webhook_deliveries (next_attempt_at) WHERE status = 'pending';
CREATE INDEX webhook_deliveries_endpoint_created ON webhook_deliveries (endpoint_id, created_at DESC);
