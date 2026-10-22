-- Manual rollback of migrations/004_town.sql (NOT embedded, never applied by the relay). Run it by hand, once,
-- with the relay scaled to 0, before starting the mountain image again (psql -1 -f relay/rollback/004_town_down.sql):
-- it puts the mountain's history back and forgets the town's (its rows are kept in town_archive_*).
CREATE TABLE town_archive_events AS TABLE events;
CREATE TABLE town_archive_world_deltas AS TABLE world_deltas;
CREATE TABLE town_archive_snapshots AS TABLE snapshots;
CREATE TABLE town_archive_metrics_5m AS TABLE metrics_5m;
DELETE FROM events;
DELETE FROM world_deltas;
DELETE FROM snapshots;
DELETE FROM metrics_5m;
INSERT INTO events SELECT * FROM mountain_archive_events;
INSERT INTO world_deltas SELECT * FROM mountain_archive_world_deltas;
INSERT INTO snapshots SELECT * FROM mountain_archive_snapshots;
INSERT INTO metrics_5m SELECT * FROM mountain_archive_metrics_5m;
UPDATE marks SET deleted_at = NULL WHERE id IN (SELECT id FROM mountain_archive_marks_hidden); -- the toys' marks 004 hid
DROP TABLE mountain_archive_events, mountain_archive_world_deltas, mountain_archive_snapshots, mountain_archive_metrics_5m,
  mountain_archive_marks_hidden, town_epoch;
DELETE FROM schema_migrations WHERE version = '004_town.sql';
