-- W4 visitor marks (02, 06, 08): the private record of every accepted mark: who (OIDC sub + the handle shown), which
-- toy, the sanitised public pose and weight. The public side rides world:mark frames (world_deltas, hello.state.marks);
-- this table holds what never leaves the relay: the rate limits (1/min, 20 per 24 h per sub, seeded from here at boot)
-- and moderation (admin mark.delete sets deleted_at). ponytail: no limits table, the limits derive from these rows.
CREATE TABLE marks (
  id         text PRIMARY KEY,
  sub        text NOT NULL,
  handle     text NOT NULL,
  toy        text NOT NULL,
  ts         timestamptz NOT NULL,
  day        date NOT NULL,
  pose       jsonb NOT NULL,
  weight     double precision NOT NULL,
  deleted_at timestamptz
);
CREATE INDEX marks_sub_ts ON marks (sub, ts);
