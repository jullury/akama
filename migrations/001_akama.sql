CREATE TABLE IF NOT EXISTS akama_jobs (
    id                 BIGSERIAL PRIMARY KEY,
    chat_id            TEXT   NOT NULL,
    platform           TEXT   NOT NULL DEFAULT 'telegram',
    provider           TEXT   NOT NULL,  -- 'github' | 'gitlab' | 'trello'
    repo_url           TEXT   NOT NULL,
    issue_id           TEXT   NOT NULL,
    issue_title        TEXT   NOT NULL,
    issue_body         TEXT   NOT NULL DEFAULT '',
    status             TEXT   NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','running','pr_created','updating','done','failed')),
    branch_name        TEXT   NOT NULL DEFAULT '',
    pr_url             TEXT   NOT NULL DEFAULT '',
    workspace_path     TEXT   NOT NULL DEFAULT '',
    notification_msg_id TEXT  NOT NULL DEFAULT '',
    agent              TEXT   NOT NULL DEFAULT 'opencode',
    agent_model        TEXT   NOT NULL DEFAULT '',
    agent_output       TEXT   NOT NULL DEFAULT '',
    error_msg          TEXT   NOT NULL DEFAULT '',
    git_token          TEXT   NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS akama_conversations (
    chat_id     TEXT NOT NULL,
    platform    TEXT NOT NULL DEFAULT 'telegram',
    state       TEXT NOT NULL DEFAULT 'idle',
    data        JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, platform)
);

CREATE INDEX IF NOT EXISTS idx_akama_jobs_chat ON akama_jobs(chat_id, platform, status);
CREATE INDEX IF NOT EXISTS idx_akama_jobs_notif ON akama_jobs(notification_msg_id);
