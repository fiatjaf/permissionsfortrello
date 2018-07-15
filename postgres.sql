CREATE TABLE boards (
  id text PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  token text,

  CHECK (id != '')
);

CREATE TABLE backups (
  id text PRIMARY KEY,
  board text,
  data jsonb NOT NULL,

  CHECK (id != '')
);

table boards;
table backups;
delete from backups;

WITH
init AS (
  SELECT ('{"id":"5b4a910f0aa8cfb949529fb9","name":"Checklist"}' || '{"idCheckItems": []}'::jsonb || data) AS data
  FROM (
    SELECT 0 AS idx, data FROM backups WHERE id = '5b4a910f0aa8cfb949529fb9'
    UNION ALL
    SELECT 1 AS idx, '{}'::jsonb
  ) AS whatever
  ORDER BY idx LIMIT 1
),
new AS (
  SELECT jsonb_set(
    data,
    '{idCheckItems}',
    data->'idCheckItems' || '"5b4a9232a495da35ccca80ea"'
  ) AS data
    FROM init
)
INSERT INTO backups (id, board, data) VALUES ('5b4a910f0aa8cfb949529fb9', '5b1980e92c9e71f5e06ad718', (SELECT data FROM new))
  ON CONFLICT (id) DO UPDATE
    SET data = (SELECT data FROM new);
