-- Typed projection of records.data. JSONB remains the source of truth.
-- Equality, range, emptiness, and sort use these btree indexes, which stay
-- one set of indexes no matter how many fields a section adds.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE record_values(
  record_id text NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  section_id text NOT NULL REFERENCES sections(id),
  field_id text NOT NULL,
  text_value text,
  number_value double precision,
  date_value date,
  bool_value boolean,
  PRIMARY KEY(record_id, field_id)
);
CREATE INDEX record_values_text ON record_values(section_id, field_id, text_value, record_id);
CREATE INDEX record_values_number ON record_values(section_id, field_id, number_value, record_id);
CREATE INDEX record_values_date ON record_values(section_id, field_id, date_value, record_id);
CREATE INDEX record_values_bool ON record_values(section_id, field_id, bool_value, record_id);
CREATE INDEX record_values_text_trgm ON record_values USING GIN (text_value gin_trgm_ops);
CREATE INDEX records_section_updated_id ON records(section_id, updated_at DESC, id);

-- Timeline entries survive record edits. type is an open string.
-- A future comment is a row with type 'comment' and an author; mentions are not stored yet.
CREATE TABLE activities(
  id text PRIMARY KEY,
  section_id text NOT NULL REFERENCES sections(id),
  record_id text NOT NULL,
  type text NOT NULL,
  occurred_at timestamptz NOT NULL,
  summary text NOT NULL,
  channel text,
  ref text,
  author_kind text NOT NULL CHECK (author_kind IN ('user', 'agent')),
  author_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activities_record ON activities(section_id, record_id, occurred_at DESC, id DESC);

-- One source record can be linked into a target section only once.
CREATE TABLE record_links(
  id text PRIMARY KEY,
  source_section_id text NOT NULL REFERENCES sections(id),
  source_record_id text NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  target_section_id text NOT NULL REFERENCES sections(id),
  target_record_id text NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_section_id, source_record_id, target_section_id)
);
CREATE INDEX record_links_target ON record_links(target_section_id, target_record_id);

INSERT INTO record_values(record_id, section_id, field_id, text_value, number_value, date_value, bool_value)
SELECT r.id, r.section_id, f.id,
  CASE WHEN f.type IN ('text', 'email') THEN r.data->>f.id END,
  CASE WHEN f.type = 'number' THEN (r.data->>f.id)::double precision END,
  CASE WHEN f.type = 'date' THEN (r.data->>f.id)::date END,
  CASE WHEN f.type = 'boolean' THEN (r.data->>f.id)::boolean END
FROM records r
JOIN fields f ON f.section_id = r.section_id
WHERE r.data ? f.id
  AND jsonb_typeof(r.data->f.id) <> 'null'
  AND (
    (f.type IN ('text', 'email') AND jsonb_typeof(r.data->f.id) = 'string' AND btrim(r.data->>f.id) <> '')
    OR (f.type = 'number' AND jsonb_typeof(r.data->f.id) = 'number')
    OR (f.type = 'date' AND (r.data->>f.id) ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$')
    OR (f.type = 'boolean' AND jsonb_typeof(r.data->f.id) = 'boolean')
  );
