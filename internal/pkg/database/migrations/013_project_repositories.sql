-- Repositories whose code project members chat about, as Sourcebot names them (github.com/org/repo).
ALTER TABLE projects ADD COLUMN repositories TEXT[] NOT NULL DEFAULT '{}';
