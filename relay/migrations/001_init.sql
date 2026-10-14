-- W2 persistence (02 "Persistence"). marks and limits arrive with W4; snapshot veins/glacier/ash/clouds
-- columns arrive with their models (W3) in a later migration; events.detail arrives with detail mode (08).
-- One sequence orders every frame and snapshot: the relay writes them from one goroutine, in broadcast order.
CREATE SEQUENCE frame_seq;

-- every broadcast ev frame, as sent (sanitised public frame, never raw input)
CREATE TABLE events (
  seq    bigint PRIMARY KEY DEFAULT nextval('frame_seq'),
  id     text NOT NULL UNIQUE,
  ts     timestamptz NOT NULL,
  day    date NOT NULL,
  kind   text NOT NULL,
  public jsonb NOT NULL
);
CREATE INDEX events_day ON events (day);

-- every broadcast world frame, as sent
CREATE TABLE world_deltas (
  seq   bigint PRIMARY KEY DEFAULT nextval('frame_seq'),
  ts    timestamptz NOT NULL,
  day   date NOT NULL,
  kind  text NOT NULL,
  frame jsonb NOT NULL
);
CREATE INDEX world_deltas_day ON world_deltas (day);

-- one row per UTC day, upserted every 10 min, at rollover and at shutdown. seq: frames after it are not in it.
-- land: h.land as little-endian int16 (h.base is Go-generated from seed, never stored).
CREATE TABLE snapshots (
  day     date PRIMARY KEY,
  seq     bigint NOT NULL,
  ts      timestamptz NOT NULL,
  seed    bigint NOT NULL,
  land    bytea NOT NULL,
  state   jsonb NOT NULL,
  metrics jsonb NOT NULL
);

-- the metrics frame every 5 min (scrub replays)
CREATE TABLE metrics_5m (
  ts    timestamptz PRIMARY KEY,
  day   date NOT NULL,
  frame jsonb NOT NULL
);
CREATE INDEX metrics_5m_day ON metrics_5m (day);
