ALTER TABLE deployments ADD COLUMN project_name text NOT NULL DEFAULT '';
CREATE INDEX idx_deployments_project_name ON deployments(project_name);
