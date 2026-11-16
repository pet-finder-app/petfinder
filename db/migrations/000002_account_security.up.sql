ALTER TABLE users ADD COLUMN auth_version integer NOT NULL DEFAULT 0;

ALTER TABLE refresh_sessions ADD COLUMN auth_version integer NOT NULL DEFAULT 0;
