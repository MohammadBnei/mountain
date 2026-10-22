-- R7 town: a fresh start. The mountain's history (terrain snapshots, deposit / vein / glacier frames, its events
-- and metrics) moves to mountain_archive_* tables, same database, same owner: the town starts empty, nothing is
-- lost, and relay/rollback/004_town_down.sql moves it back for a revert to the mountain image. The mountain toys'
-- marks are soft-deleted (deleted_at: their rows keep the per-account rate-limit history honest). town_epoch is
-- the town's first day: a scrub of an earlier day answers "unavailable" (scrub.go).
CREATE TABLE mountain_archive_events AS TABLE events;
CREATE TABLE mountain_archive_world_deltas AS TABLE world_deltas;
CREATE TABLE mountain_archive_snapshots AS TABLE snapshots;
CREATE TABLE mountain_archive_metrics_5m AS TABLE metrics_5m;
DELETE FROM events;
DELETE FROM world_deltas;
DELETE FROM snapshots;
DELETE FROM metrics_5m;
CREATE TABLE mountain_archive_marks_hidden AS SELECT id FROM marks WHERE toy <> 'plant' AND deleted_at IS NULL;
UPDATE marks SET deleted_at = now() WHERE id IN (SELECT id FROM mountain_archive_marks_hidden);
CREATE TABLE town_epoch (started date NOT NULL);
INSERT INTO town_epoch VALUES ((now() AT TIME ZONE 'UTC')::date);
