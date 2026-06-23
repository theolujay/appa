DROP INDEX IF EXISTS idx_deployments_project_name;
ALTER TABLE deployments DROP COLUMN IF EXISTS project_name;
