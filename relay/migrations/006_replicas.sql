-- R8 deploy C1 (infra-bootstrap ADR-0065): the replica buckets are coarse now (replicaB: 0, 1, 2..4, 5..7, 8..15,
-- 16+), so read with an app's annexes they give no exact small counts. History gets the same buckets, or a replay of
-- the last days would still show them: every old bucket lies inside exactly one new one (TestReplicaB), so the
-- rewrite is exact and needs no raw count. One-way: there is no down script (a revert of deploy C1 keeps the coarse
-- history, which stays valid data for any relay: fBucket only narrows).
--   old 0 1 2 3 4 5 6 7 8
--   new 0 1 2 2 2 3 4 5 5
-- events (ev replicas) and the snapshots' state.replicas; r7_archive_day_snapshot too (005's rollback copy).
CREATE OR REPLACE FUNCTION pg_temp.r8c_bucket(b jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN jsonb_typeof(b) = 'number' THEN
    to_jsonb(CASE (b #>> '{}')::int WHEN 3 THEN 2 WHEN 4 THEN 2 WHEN 5 THEN 3 WHEN 6 THEN 4 WHEN 7 THEN 5 WHEN 8 THEN 5
      ELSE (b #>> '{}')::int END)
  ELSE b END
$$;
UPDATE events SET public = jsonb_set(public, '{bucket}', pg_temp.r8c_bucket(public -> 'bucket'))
  WHERE kind = 'replicas' AND public ? 'bucket';
UPDATE snapshots SET state = jsonb_set(state, '{replicas}',
    (SELECT coalesce(jsonb_object_agg(k, pg_temp.r8c_bucket(v)), '{}'::jsonb) FROM jsonb_each(state -> 'replicas') AS e(k, v)))
  WHERE jsonb_typeof(state -> 'replicas') = 'object';
UPDATE r7_archive_day_snapshot SET state = jsonb_set(state, '{replicas}',
    (SELECT coalesce(jsonb_object_agg(k, pg_temp.r8c_bucket(v)), '{}'::jsonb) FROM jsonb_each(state -> 'replicas') AS e(k, v)))
  WHERE jsonb_typeof(state -> 'replicas') = 'object';
