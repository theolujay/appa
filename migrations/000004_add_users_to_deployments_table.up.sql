ALTER TABLE deployments ADD COLUMN user_id bigint REFERENCES users(id);
ALTER TABLE deployments ALTER COLUMN user_id SET NOT NULL;