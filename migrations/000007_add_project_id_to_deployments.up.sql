ALTER TABLE deployments ADD COLUMN project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL;
CREATE INDEX idx_deployments_project_id ON deployments(project_id);