CREATE TABLE boards (
  id text PRIMARY KEY,
  token text NOT NULL,
  email text NOT NULL,
  webhook_id text NOT NULL,

  CHECK (id != ''),
  CHECK (token != ''),
  CHECK (email != ''),
  CHECK (webhook_id != '')
);

CREATE TABLE backups (
  id text PRIMARY KEY,
  board text,
  data jsonb NOT NULL,

  CHECK (id != ''),
  CHECK (board != '')
);

table boards;
select id from backups;
