CREATE TABLE comment_access_grant (
    grant_hash BYTEA PRIMARY KEY,
    issuer VARCHAR(64) NOT NULL,
    user_id UUID NOT NULL,
    space_id UUID NOT NULL REFERENCES comment_space (id) ON DELETE CASCADE,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(512) NOT NULL,
    permissions SMALLINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_comment_access_grant_hash CHECK (octet_length(grant_hash) = 32),
    CONSTRAINT chk_comment_access_grant_issuer
        CHECK (issuer ~ '^[a-z0-9][a-z0-9._-]{1,63}$'),
    CONSTRAINT chk_comment_access_grant_resource_type
        CHECK (resource_type ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
    CONSTRAINT chk_comment_access_grant_resource_id
        CHECK (char_length(btrim(resource_id)) > 0),
    CONSTRAINT chk_comment_access_grant_permissions
        CHECK (
            permissions BETWEEN 1 AND 7
            AND (permissions & 1) = 1
            AND ((permissions & 4) = 0 OR (permissions & 2) = 2)
        ),
    CONSTRAINT chk_comment_access_grant_expiry CHECK (expires_at > created_at)
);

CREATE INDEX idx_comment_access_grant_expiry
    ON comment_access_grant (expires_at, grant_hash);
