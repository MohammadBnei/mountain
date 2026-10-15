-- Detail mode (08): the unsanitised detail frame of an event, sent to platform-admins only; cleared after 30 days (02).
ALTER TABLE events ADD COLUMN detail jsonb;
