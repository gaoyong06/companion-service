-- Companion PostgreSQL schema. Requires PostgreSQL 15+ and pgvector.
CREATE EXTENSION IF NOT EXISTS vector;

-- 会话主表，记录用户与陪伴角色之间的对话容器。
CREATE TABLE IF NOT EXISTS companion_conversation (
    conversation_id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    companion_id VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    onboarding_stage VARCHAR(32) NOT NULL DEFAULT 'first_meeting',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE companion_conversation ADD COLUMN IF NOT EXISTS onboarding_stage VARCHAR(32) NOT NULL DEFAULT 'first_meeting';
COMMENT ON TABLE companion_conversation IS '会话主表';
COMMENT ON COLUMN companion_conversation.conversation_id IS '会话唯一标识，由服务端生成，通常使用 conv_ 前缀';
COMMENT ON COLUMN companion_conversation.user_id IS '所属用户唯一标识，用于用户数据隔离和访问控制';
COMMENT ON COLUMN companion_conversation.companion_id IS '陪伴角色唯一标识，当前默认值为 default';
COMMENT ON COLUMN companion_conversation.status IS '内部上下文段状态：active 表示当前使用，closed 表示已由系统分段';
COMMENT ON COLUMN companion_conversation.summary IS '内部上下文段摘要，由系统在上下文预算达到阈值时生成';
COMMENT ON COLUMN companion_conversation.onboarding_stage IS '服务端维护的破冰阶段：first_meeting、small_talk、getting_to_know、trust 或 established';
COMMENT ON COLUMN companion_conversation.created_at IS '会话创建时间，使用 UTC';
COMMENT ON COLUMN companion_conversation.updated_at IS '会话最后更新时间，使用 UTC';
CREATE INDEX IF NOT EXISTS idx_companion_conversation_user ON companion_conversation (user_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_companion_active_conversation
    ON companion_conversation (user_id, companion_id)
    WHERE status = 'active';

-- 消息表，保存会话中的用户消息和陪伴回复。
CREATE TABLE IF NOT EXISTS companion_message (
    message_id VARCHAR(64) PRIMARY KEY,
    conversation_id VARCHAR(64) NOT NULL REFERENCES companion_conversation(conversation_id),
    user_id VARCHAR(64) NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    modality VARCHAR(16) NOT NULL DEFAULT 'text',
    created_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE companion_message ADD COLUMN IF NOT EXISTS modality VARCHAR(16) NOT NULL DEFAULT 'text';
COMMENT ON TABLE companion_message IS '会话消息表';
COMMENT ON COLUMN companion_message.message_id IS '消息唯一标识，由服务端生成，通常使用 msg_ 前缀';
COMMENT ON COLUMN companion_message.conversation_id IS '所属会话唯一标识';
COMMENT ON COLUMN companion_message.user_id IS '所属用户唯一标识，冗余保存以支持用户范围查询和数据治理';
COMMENT ON COLUMN companion_message.role IS '消息角色：user、assistant 或 system';
COMMENT ON COLUMN companion_message.content IS '消息正文，使用 UTF-8 文本保存';
COMMENT ON COLUMN companion_message.modality IS '消息载体类型：text、audio、image 或 video';
COMMENT ON COLUMN companion_message.created_at IS '消息创建时间，使用 UTC';
CREATE INDEX IF NOT EXISTS idx_companion_message_conversation ON companion_message (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_companion_message_user ON companion_message (user_id, created_at);

-- 消息资产关联表，保存图片或视频在 asset-service 中的引用，不保存二进制内容。
CREATE TABLE IF NOT EXISTS companion_message_asset (
    message_asset_id VARCHAR(64) PRIMARY KEY,
    message_id VARCHAR(64) NOT NULL REFERENCES companion_message(message_id),
    asset_id VARCHAR(64) NOT NULL,
    media_type VARCHAR(16) NOT NULL,
    content_type VARCHAR(128) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0)
);
COMMENT ON TABLE companion_message_asset IS '消息媒体资产关联表，实际文件由 asset-service 管理';
COMMENT ON COLUMN companion_message_asset.message_asset_id IS '消息资产关联唯一标识';
COMMENT ON COLUMN companion_message_asset.message_id IS '所属消息唯一标识';
COMMENT ON COLUMN companion_message_asset.asset_id IS 'asset-service 生成的文件唯一标识';
COMMENT ON COLUMN companion_message_asset.media_type IS '媒体类型：image 或 video';
COMMENT ON COLUMN companion_message_asset.content_type IS '文件 MIME 类型';
COMMENT ON COLUMN companion_message_asset.filename IS '用户上传时的原始文件名';
COMMENT ON COLUMN companion_message_asset.url IS '资产访问地址';
COMMENT ON COLUMN companion_message_asset.size_bytes IS '文件大小，单位为字节';
CREATE INDEX IF NOT EXISTS idx_companion_message_asset_message ON companion_message_asset (message_id);
CREATE INDEX IF NOT EXISTS idx_companion_message_asset_asset ON companion_message_asset (asset_id);

-- 记忆表，保存从用户对话中抽取并用于上下文召回的记忆。
CREATE TABLE IF NOT EXISTS companion_memory (
    memory_id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    layer VARCHAR(16) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    content TEXT NOT NULL,
    source_message_id VARCHAR(64) NOT NULL,
    confidence NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    importance INTEGER NOT NULL CHECK (importance BETWEEN 1 AND 5),
    status VARCHAR(16) NOT NULL,
    embedding vector(1536),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE companion_memory IS '用户记忆表，向量用于语义召回';
COMMENT ON COLUMN companion_memory.memory_id IS '记忆唯一标识，由服务端生成，通常使用 mem_ 前缀';
COMMENT ON COLUMN companion_memory.user_id IS '所属用户唯一标识，记忆只能在该用户上下文中召回';
COMMENT ON COLUMN companion_memory.layer IS '记忆层级，当前使用 L1';
COMMENT ON COLUMN companion_memory.kind IS '记忆类型：preference、fact 或 goal';
COMMENT ON COLUMN companion_memory.content IS '记忆内容，不得保存密码、令牌、银行卡等敏感信息';
COMMENT ON COLUMN companion_memory.source_message_id IS '产生该记忆的源消息唯一标识';
COMMENT ON COLUMN companion_memory.confidence IS '模型对记忆内容正确性的置信度，范围 0 至 1';
COMMENT ON COLUMN companion_memory.importance IS '记忆重要性评分，范围 1 至 5';
COMMENT ON COLUMN companion_memory.status IS '记忆状态：active 表示可召回，deleted 表示不可召回';
COMMENT ON COLUMN companion_memory.embedding IS '记忆内容的 1536 维向量，用于 pgvector 相似度检索';
COMMENT ON COLUMN companion_memory.created_at IS '记忆首次创建时间，使用 UTC';
COMMENT ON COLUMN companion_memory.updated_at IS '记忆最后更新时间，使用 UTC';
CREATE INDEX IF NOT EXISTS idx_companion_memory_user_status ON companion_memory (user_id, status, importance, updated_at);
CREATE INDEX IF NOT EXISTS idx_companion_memory_source ON companion_memory (source_message_id);
CREATE INDEX IF NOT EXISTS idx_companion_memory_embedding
    ON companion_memory USING hnsw (embedding vector_cosine_ops)
    WHERE status = 'active';
