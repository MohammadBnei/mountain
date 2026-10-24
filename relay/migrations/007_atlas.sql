-- R10 A1 (plan/10-understanding.md "Read side"): atlas's projected graph, versioned. Additive only: the town's
-- tables are untouched while it still runs (A2 drops them in the same deploy that replaces the town).
--
-- A node or edge version is valid for valid_from <= seq < valid_to (NULL: still current). A projection that changes
-- nothing writes nothing, so seq advances only when the system's structure did. History is never pruned by deleting
-- projections (RESTRICT): that would reopen or drop versions; a fixed extractor re-projects into a new epoch instead.
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
