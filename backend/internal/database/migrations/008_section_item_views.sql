-- Info is an application invariant, not a removable database row.
CREATE TABLE section_item_views (
    section_id text NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
    view_id text NOT NULL CHECK (view_id <> 'info' AND view_id ~ '^[a-z][a-z0-9_]{0,47}$'),
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    position integer NOT NULL CHECK (position >= 0),
    PRIMARY KEY (section_id, view_id)
);
