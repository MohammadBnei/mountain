-- R8: islands sized by host and a building typology. Local hex coordinates keep their meaning (an R7 plot is still
-- on its island), so the town keeps its epoch and its history; but every plot is re-placed once, by its family
-- (relay/town Slot). town_r8.started marks the cut: restore drops the plots of a snapshot written before it, and
-- the first poll places them again (store.go restore). r7_archive_day_snapshot keeps the deploy day's R7.1 row,
-- which R8's own snapshot overwrites (snapshots are keyed by day): relay/rollback/005_r8_down.sql puts it back.
CREATE TABLE town_r8 (started timestamptz NOT NULL);
INSERT INTO town_r8 VALUES (now());
CREATE TABLE r7_archive_day_snapshot AS SELECT * FROM snapshots WHERE day = (now() AT TIME ZONE 'UTC')::date;
