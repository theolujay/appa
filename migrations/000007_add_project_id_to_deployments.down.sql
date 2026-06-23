
DROP INDEX IF EXISTS idx_deployments_project_id;
ALTER TABLE deployments DROP COLUMN IF EXISTS project_id;
