CREATE TABLE IF NOT EXISTS deployments (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    image_tag TEXT,
    address TEXT,
    env_vars TEXT,
    url TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);