-- Manual rollback of migrations/005_r8.sql (NOT embedded, never applied by the relay). Run it by hand, with ArgoCD
-- auto-sync off and the relay scaled to 0, BEFORE the R7.1 image starts again (plan/README.md "Rollback";
-- psql -1 -f relay/rollback/005_r8_down.sql). R8's world frames and snapshots can name building kinds and hexes
-- an R7.1 relay does not know (rings 4..7): they move to r8_archive_* (added to, so it can run twice), the deploy
-- day's R7.1 snapshot comes back, and the plants planted since R8 are hidden (deleted_at: their rows keep the
-- rate limits honest). Events and metrics are the cluster's, the same in both: they stay. To go back further, run
-- this first, then 004_town_down.sql.
CREATE TABLE IF NOT EXISTS r8_archive_world_deltas AS TABLE world_deltas WITH NO DATA;
CREATE TABLE IF NOT EXISTS r8_archive_snapshots AS TABLE snapshots WITH NO DATA;
CREATE TABLE IF NOT EXISTS r8_archive_marks_hidden (id text PRIMARY KEY);
INSERT INTO r8_archive_world_deltas SELECT * FROM world_deltas WHERE ts >= (SELECT started FROM town_r8);
INSERT INTO r8_archive_snapshots SELECT * FROM snapshots WHERE ts >= (SELECT started FROM town_r8);
DELETE FROM world_deltas WHERE ts >= (SELECT started FROM town_r8);
DELETE FROM snapshots WHERE ts >= (SELECT started FROM town_r8);
INSERT INTO snapshots SELECT * FROM r7_archive_day_snapshot ON CONFLICT (day) DO NOTHING;
INSERT INTO r8_archive_marks_hidden SELECT id FROM marks WHERE ts >= (SELECT started FROM town_r8) AND deleted_at IS NULL
  ON CONFLICT DO NOTHING;
UPDATE marks SET deleted_at = now() WHERE ts >= (SELECT started FROM town_r8) AND deleted_at IS NULL;
DROP TABLE town_r8, r7_archive_day_snapshot;
DELETE FROM schema_migrations WHERE version = '005_r8.sql';
