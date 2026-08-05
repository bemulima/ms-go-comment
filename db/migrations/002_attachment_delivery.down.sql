DROP INDEX idx_comment_attachment_delete_work;
DROP INDEX idx_comment_attachment_activation_work;

ALTER TABLE comment_attachment
    DROP CONSTRAINT chk_comment_attachment_delete_attempts,
    DROP CONSTRAINT chk_comment_attachment_activation_attempts,
    DROP COLUMN storage_deleted_at,
    DROP COLUMN last_error,
    DROP COLUMN delete_next_attempt_at,
    DROP COLUMN delete_attempts,
    DROP COLUMN activation_next_attempt_at,
    DROP COLUMN activation_attempts;
