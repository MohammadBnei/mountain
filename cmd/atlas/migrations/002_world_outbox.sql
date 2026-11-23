-- World events (the bridge's dev.bnei.world.*): atlas publishes the world's births, bonds and retirements, so the
-- library can build arcs keyed by System id. The outbox is written in projectGraph's transaction (the diff of the
-- previous projection and this one), and the publisher drains it to the bridge; the database stays rebuildable.
--
-- The epoch is the first projection's time in unix nanos: a ce_id carries <epoch>.<seq>, so ids grow across a rebuilt
-- database as well as within one. A fresh database has no row until its first projection writes it.
CREATE TABLE atlas_epoch (epoch bigint NOT NULL);
INSERT INTO atlas_epoch SELECT (extract(epoch FROM min(at)) * 1000000000)::bigint FROM atlas_projection HAVING count(*) > 0;

CREATE TABLE atlas_outbox (
  id      bigserial PRIMARY KEY,
  at      timestamptz NOT NULL DEFAULT now(),
  ce_id   text        NOT NULL UNIQUE,
  type    text        NOT NULL,
  subject text        NOT NULL,
  data    jsonb       NOT NULL,
  time    timestamptz NOT NULL -- the projection's time: the event's
);

-- An existing database gets one baseline: the library re-snapshots atlas at the latest projection.
INSERT INTO atlas_outbox (ce_id, type, subject, data, time)
SELECT 'rebased:' || e.epoch || '.' || p.seq, 'dev.bnei.world.history.rebased', 'world/',
       jsonb_build_object('epoch', e.epoch, 'seq', p.seq, 'cause', 'baseline',
         'vector', (SELECT coalesce(jsonb_object_agg(v.repo, v.branch || '@' || v.sha), '{}') FROM atlas_vector v WHERE v.seq = p.seq)),
       p.at
FROM atlas_epoch e, (SELECT seq, at FROM atlas_projection ORDER BY seq DESC LIMIT 1) p;
