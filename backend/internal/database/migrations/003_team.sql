ALTER TABLE users ADD COLUMN name text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'admin';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'member'));
ALTER TABLE users ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE audit ADD COLUMN detail text;
CREATE TABLE invites(
  id text PRIMARY KEY,
  email text NOT NULL UNIQUE,
  role text NOT NULL CHECK (role IN ('admin', 'member')),
  token_hash text NOT NULL UNIQUE,
  created_by text REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);
