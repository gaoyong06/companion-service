CREATE TABLE companion_conversation (
    conversation_id VARCHAR(64) NOT NULL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    companion_id VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    summary TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    KEY idx_companion_conversation_user (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE companion_message (
    message_id VARCHAR(64) NOT NULL PRIMARY KEY,
    conversation_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    KEY idx_companion_message_conversation (conversation_id, created_at),
    KEY idx_companion_message_user (user_id, created_at),
    CONSTRAINT fk_companion_message_conversation
        FOREIGN KEY (conversation_id) REFERENCES companion_conversation (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
