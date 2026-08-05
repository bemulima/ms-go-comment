CREATE TABLE comment_space (
    id UUID PRIMARY KEY,
    key VARCHAR(64) NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,
    access_mode SMALLINT NOT NULL DEFAULT 1,
    allowed_origins TEXT[] NOT NULL DEFAULT '{}',
    allow_images BOOLEAN NOT NULL DEFAULT FALSE,
    allow_links BOOLEAN NOT NULL DEFAULT TRUE,
    max_depth SMALLINT NOT NULL DEFAULT 10,
    max_body_length INTEGER NOT NULL DEFAULT 10000,
    max_attachments SMALLINT NOT NULL DEFAULT 4,
    max_image_bytes BIGINT NOT NULL DEFAULT 5242880,
    edit_window_seconds INTEGER NOT NULL DEFAULT 900,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_comment_space_key
        CHECK (key ~ '^[a-z0-9][a-z0-9._-]{1,63}$'),
    CONSTRAINT chk_comment_space_name CHECK (char_length(btrim(name)) > 0),
    CONSTRAINT chk_comment_space_status CHECK (status IN (0, 1)),
    CONSTRAINT chk_comment_space_access_mode CHECK (access_mode IN (1, 2)),
    CONSTRAINT chk_comment_space_max_depth CHECK (max_depth BETWEEN 1 AND 32),
    CONSTRAINT chk_comment_space_max_body_length CHECK (max_body_length BETWEEN 1 AND 100000),
    CONSTRAINT chk_comment_space_max_attachments CHECK (max_attachments BETWEEN 0 AND 10),
    CONSTRAINT chk_comment_space_max_image_bytes CHECK (max_image_bytes BETWEEN 1 AND 26214400),
    CONSTRAINT chk_comment_space_edit_window CHECK (edit_window_seconds BETWEEN 0 AND 604800)
);

CREATE TABLE comment_thread (
    id UUID PRIMARY KEY,
    space_id UUID NOT NULL REFERENCES comment_space (id) ON DELETE RESTRICT,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(512) NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,
    allow_images BOOLEAN,
    allow_links BOOLEAN,
    max_depth SMALLINT,
    max_body_length INTEGER,
    max_attachments SMALLINT,
    max_image_bytes BIGINT,
    edit_window_seconds INTEGER,
    comment_count BIGINT NOT NULL DEFAULT 0,
    root_comment_count BIGINT NOT NULL DEFAULT 0,
    last_sequence BIGINT NOT NULL DEFAULT 0,
    last_comment_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_comment_thread_resource UNIQUE (space_id, resource_type, resource_id),
    CONSTRAINT chk_comment_thread_resource_type
        CHECK (resource_type ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
    CONSTRAINT chk_comment_thread_resource_id CHECK (char_length(btrim(resource_id)) > 0),
    CONSTRAINT chk_comment_thread_status CHECK (status IN (1, 2, 3, 4)),
    CONSTRAINT chk_comment_thread_max_depth CHECK (max_depth IS NULL OR max_depth BETWEEN 1 AND 32),
    CONSTRAINT chk_comment_thread_max_body_length
        CHECK (max_body_length IS NULL OR max_body_length BETWEEN 1 AND 100000),
    CONSTRAINT chk_comment_thread_max_attachments
        CHECK (max_attachments IS NULL OR max_attachments BETWEEN 0 AND 10),
    CONSTRAINT chk_comment_thread_max_image_bytes
        CHECK (max_image_bytes IS NULL OR max_image_bytes BETWEEN 1 AND 26214400),
    CONSTRAINT chk_comment_thread_edit_window
        CHECK (edit_window_seconds IS NULL OR edit_window_seconds BETWEEN 0 AND 604800),
    CONSTRAINT chk_comment_thread_counters
        CHECK (comment_count >= 0 AND root_comment_count >= 0 AND root_comment_count <= comment_count),
    CONSTRAINT chk_comment_thread_sequence CHECK (last_sequence >= 0)
);

CREATE INDEX idx_comment_thread_space_status
    ON comment_thread (space_id, status, updated_at DESC, id);

CREATE TABLE comment (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL REFERENCES comment_thread (id) ON DELETE RESTRICT,
    author_id UUID NOT NULL,
    parent_id UUID,
    root_id UUID NOT NULL,
    path UUID[] NOT NULL,
    depth SMALLINT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    links JSONB NOT NULL DEFAULT '[]'::JSONB,
    status SMALLINT NOT NULL DEFAULT 1,
    version INTEGER NOT NULL DEFAULT 1,
    sequence BIGINT NOT NULL,
    direct_replies_count BIGINT NOT NULL DEFAULT 0,
    idempotency_key UUID NOT NULL,
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_comment_thread_id_id UNIQUE (thread_id, id),
    CONSTRAINT uq_comment_author_idempotency UNIQUE (author_id, idempotency_key),
    CONSTRAINT fk_comment_parent_same_thread
        FOREIGN KEY (thread_id, parent_id)
        REFERENCES comment (thread_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_comment_root_same_thread
        FOREIGN KEY (thread_id, root_id)
        REFERENCES comment (thread_id, id)
        ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT chk_comment_depth CHECK (depth BETWEEN 0 AND 32),
    CONSTRAINT chk_comment_status CHECK (status IN (1, 2, 3)),
    CONSTRAINT chk_comment_version CHECK (version >= 1),
    CONSTRAINT chk_comment_sequence CHECK (sequence >= 1),
    CONSTRAINT chk_comment_direct_replies_count CHECK (direct_replies_count >= 0),
    CONSTRAINT chk_comment_links_array CHECK (jsonb_typeof(links) = 'array'),
    CONSTRAINT chk_comment_path_shape
        CHECK (
            cardinality(path) = depth + 1
            AND path[1] = root_id
            AND path[cardinality(path)] = id
        ),
    CONSTRAINT chk_comment_root_shape
        CHECK (
            (parent_id IS NULL AND depth = 0 AND root_id = id AND cardinality(path) = 1)
            OR
            (parent_id IS NOT NULL AND depth > 0 AND root_id <> id)
        ),
    CONSTRAINT chk_comment_deleted_at CHECK (status <> 2 OR deleted_at IS NOT NULL)
);

CREATE INDEX idx_comment_thread_parent_cursor
    ON comment (thread_id, parent_id, created_at, id);
CREATE UNIQUE INDEX uq_comment_thread_sequence
    ON comment (thread_id, sequence);
CREATE INDEX idx_comment_thread_root
    ON comment (thread_id, root_id, created_at, id);
CREATE INDEX idx_comment_author_created
    ON comment (author_id, created_at DESC, id);
CREATE INDEX idx_comment_path_gin ON comment USING GIN (path);

CREATE TABLE comment_attachment (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL REFERENCES comment_thread (id) ON DELETE RESTRICT,
    comment_id UUID,
    uploader_id UUID NOT NULL,
    filestorage_id UUID NOT NULL UNIQUE,
    status SMALLINT NOT NULL DEFAULT 1,
    mime_type VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    original_filename TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    activated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_comment_attachment_comment_same_thread
        FOREIGN KEY (thread_id, comment_id)
        REFERENCES comment (thread_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT chk_comment_attachment_status CHECK (status IN (1, 2, 3, 4, 5)),
    CONSTRAINT chk_comment_attachment_mime
        CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    CONSTRAINT chk_comment_attachment_size CHECK (size_bytes BETWEEN 1 AND 26214400),
    CONSTRAINT chk_comment_attachment_dimensions
        CHECK (width BETWEEN 1 AND 32768 AND height BETWEEN 1 AND 32768),
    CONSTRAINT chk_comment_attachment_filename
        CHECK (char_length(btrim(original_filename)) > 0),
    CONSTRAINT chk_comment_attachment_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_comment_attachment_ready
        CHECK (status <> 3 OR (comment_id IS NOT NULL AND activated_at IS NOT NULL)),
    CONSTRAINT chk_comment_attachment_deleted CHECK (status <> 5 OR deleted_at IS NOT NULL)
);

CREATE INDEX idx_comment_attachment_comment
    ON comment_attachment (thread_id, comment_id, created_at, id);
CREATE INDEX idx_comment_attachment_uploader_status
    ON comment_attachment (uploader_id, status, created_at, id);
CREATE INDEX idx_comment_attachment_pending_expiry
    ON comment_attachment (expires_at, id)
    WHERE status IN (1, 4);

CREATE TABLE comment_outbox (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id UUID NOT NULL,
    subject VARCHAR(128) NOT NULL,
    schema_version SMALLINT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_comment_outbox_aggregate_type
        CHECK (char_length(btrim(aggregate_type)) > 0),
    CONSTRAINT chk_comment_outbox_subject
        CHECK (subject IN (
            'comment.created',
            'comment.updated',
            'comment.deleted',
            'comment.hidden',
            'comment.restored',
            'comment.attachment.ready',
            'comment.attachment.failed',
            'comment.thread.updated'
        )),
    CONSTRAINT chk_comment_outbox_schema_version CHECK (schema_version >= 1),
    CONSTRAINT chk_comment_outbox_payload CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT chk_comment_outbox_attempts CHECK (attempts >= 0)
);

CREATE INDEX idx_comment_outbox_pending
    ON comment_outbox (next_attempt_at, created_at, id)
    WHERE published_at IS NULL;
CREATE INDEX idx_comment_outbox_aggregate
    ON comment_outbox (aggregate_type, aggregate_id, created_at, id);

CREATE TABLE comment_ws_ticket (
    ticket_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL,
    thread_id UUID NOT NULL REFERENCES comment_thread (id) ON DELETE CASCADE,
    permissions SMALLINT NOT NULL,
    requested_last_sequence BIGINT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_comment_ws_ticket_hash CHECK (octet_length(ticket_hash) = 32),
    CONSTRAINT chk_comment_ws_ticket_permissions
        CHECK (permissions BETWEEN 1 AND 7 AND (permissions & 1) = 1),
    CONSTRAINT chk_comment_ws_ticket_sequence
        CHECK (requested_last_sequence IS NULL OR requested_last_sequence >= 0),
    CONSTRAINT chk_comment_ws_ticket_expiry CHECK (expires_at > created_at)
);

CREATE INDEX idx_comment_ws_ticket_expiry ON comment_ws_ticket (expires_at);
