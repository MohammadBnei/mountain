-- R10 A2 (plan/10-understanding.md "Read side"): atlas's own migration series, tracked in atlas_migrations. The town
-- is gone with no data worth keeping (R10 rows of 00: history is dropped), so its tables go first, and the relay's
-- schema_migrations with them. A relay that ran A1's 007 left atlas_* behind: they are dropped too, since every
-- projection is a pure function of the repos and is rebuilt by the next poll. There is no way back to the relay
-- image after this: its migrations would meet a schema they don't expect. A revert of A2 drops atlas_* and
-- atlas_migrations by hand first (plan/10 "A2 as built").
DROP TABLE IF EXISTS events, world_deltas, snapshots, metrics_5m, marks, town_epoch, town_r8,
  mountain_archive_events, mountain_archive_world_deltas, mountain_archive_snapshots, mountain_archive_metrics_5m,
  mountain_archive_marks_hidden, r7_archive_day_snapshot, schema_migrations,
  town_archive_events, town_archive_world_deltas, town_archive_snapshots, town_archive_metrics_5m, -- the manual rollbacks'
  r8_archive_world_deltas, r8_archive_snapshots, r8_archive_marks_hidden,
  atlas_drift, atlas_edge_ver, atlas_node_ver, atlas_vector, atlas_projection CASCADE;
DROP SEQUENCE IF EXISTS frame_seq;

-- A node or edge version is valid for valid_from <= seq < valid_to (NULL: still current). A projection that changes
-- nothing writes nothing, so seq advances only when the system's structure (or a tracked sha) did. History is never
-- pruned by deleting projections (RESTRICT): that would reopen or drop versions.
CREATE TABLE atlas_projection (
  seq        bigserial PRIMARY KEY,
  at         timestamptz NOT NULL DEFAULT now(),
  extractors jsonb NOT NULL, -- extractor -> version
  tracked    jsonb NOT NULL, -- [{repo, branch}] the ArgoCD Applications deploy from (the vector's keys)
  skipped    jsonb NOT NULL DEFAULT '[]'
);

CREATE TABLE atlas_vector (
  seq    bigint NOT NULL REFERENCES atlas_projection ON DELETE CASCADE,
  repo   text   NOT NULL,
  branch text   NOT NULL, -- the branch the checkout was on ('' detached); drift says when ArgoCD deploys another
  sha    text   NOT NULL,
  PRIMARY KEY (seq, repo)
);

CREATE TABLE atlas_node_ver (
  id         text   NOT NULL,
  hash       text   NOT NULL,
  body       jsonb  NOT NULL,
  valid_from bigint NOT NULL REFERENCES atlas_projection (seq) ON DELETE RESTRICT,
  valid_to   bigint REFERENCES atlas_projection (seq) ON DELETE RESTRICT,
  PRIMARY KEY (id, valid_from)
);
-- One current version per node: a projection deleted under it would otherwise reopen an old one.
CREATE UNIQUE INDEX atlas_node_ver_current ON atlas_node_ver (id) WHERE valid_to IS NULL;

CREATE TABLE atlas_edge_ver (
  key        text   NOT NULL, -- from, to, kind, origin
  hash       text   NOT NULL,
  body       jsonb  NOT NULL,
  valid_from bigint NOT NULL REFERENCES atlas_projection (seq) ON DELETE RESTRICT,
  valid_to   bigint REFERENCES atlas_projection (seq) ON DELETE RESTRICT,
  PRIMARY KEY (key, valid_from)
);
CREATE UNIQUE INDEX atlas_edge_ver_current ON atlas_edge_ver (key) WHERE valid_to IS NULL;

-- Drift is small and read whole: one array per projection.
CREATE TABLE atlas_drift (
  seq  bigint PRIMARY KEY REFERENCES atlas_projection ON DELETE CASCADE,
  body jsonb  NOT NULL
);
