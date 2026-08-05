ALTER TABLE comment_attachment
    ADD COLUMN activation_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN activation_next_attempt_at TIMESTAMPTZ,
    ADD COLUMN delete_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN delete_next_attempt_at TIMESTAMPTZ,
    ADD COLUMN last_error TEXT,
    ADD COLUMN storage_deleted_at TIMESTAMPTZ,
    ADD CONSTRAINT chk_comment_attachment_activation_attempts CHECK (activation_attempts >= 0),
    ADD CONSTRAINT chk_comment_attachment_delete_attempts CHECK (delete_attempts >= 0);

CREATE INDEX idx_comment_attachment_activation_work
    ON comment_attachment (activation_next_attempt_at, id)
    WHERE status = 2;

CREATE INDEX idx_comment_attachment_delete_work
    ON comment_attachment (delete_next_attempt_at, id)
    WHERE status = 5 AND storage_deleted_at IS NULL;
