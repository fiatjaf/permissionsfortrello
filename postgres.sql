CREATE TABLE boards (
  id text PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  token text,

  CHECK (id != '')
);

CREATE TABLE backups (
  id text PRIMARY KEY,
  data jsonb NOT NULL,

  CHECK (id != '')
);

table boards;
table backups;
