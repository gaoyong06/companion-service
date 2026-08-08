-- 会话主表，记录用户与陪伴角色之间的对话容器
CREATE TABLE companion_conversation (
    conversation_id VARCHAR(64) NOT NULL PRIMARY KEY COMMENT '会话唯一标识，由服务端生成，通常使用 conv_ 前缀',
    user_id VARCHAR(64) NOT NULL COMMENT '所属用户唯一标识，用于用户数据隔离和访问控制',
    companion_id VARCHAR(64) NOT NULL COMMENT '陪伴角色唯一标识；当前默认值为 default，用于支持多种陪伴角色',
    status VARCHAR(16) NOT NULL COMMENT '会话状态：active 表示进行中，closed 表示已关闭',
    summary TEXT NOT NULL COMMENT '会话摘要；关闭会话时由模型生成，用于保留长期上下文和未解决事项',
    created_at DATETIME(6) NOT NULL COMMENT '会话创建时间，使用 UTC 时间，精确到微秒',
    updated_at DATETIME(6) NOT NULL COMMENT '会话最后更新时间，使用 UTC 时间，精确到微秒',
    -- 按用户和创建时间倒序查询用户的会话列表。
    KEY idx_companion_conversation_user (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话主表';

-- 消息表，保存会话中的用户消息和陪伴回复
CREATE TABLE companion_message (
    message_id VARCHAR(64) NOT NULL PRIMARY KEY COMMENT '消息唯一标识，由服务端生成，通常使用 msg_ 前缀',
    conversation_id VARCHAR(64) NOT NULL COMMENT '所属会话唯一标识，关联 companion_conversation.conversation_id',
    user_id VARCHAR(64) NOT NULL COMMENT '所属用户唯一标识；冗余保存以支持用户范围查询和数据删除',
    role VARCHAR(16) NOT NULL COMMENT '消息角色：user 表示用户消息，assistant 表示陪伴回复；system 仅用于模型上下文，不应由用户直接写入',
    content TEXT NOT NULL COMMENT '消息正文；用户输入和模型生成内容均以 UTF-8 文本保存',
    created_at DATETIME(6) NOT NULL COMMENT '消息创建时间，使用 UTC 时间，精确到微秒',
    -- 按会话和创建时间查询上下文历史。
    KEY idx_companion_message_conversation (conversation_id, created_at),
    -- 按用户和创建时间查询用户数据及执行删除操作。
    KEY idx_companion_message_user (user_id, created_at),
    -- 约束消息必须属于已存在的会话；删除会话时由服务事务先处理消息。
    CONSTRAINT fk_companion_message_conversation
        FOREIGN KEY (conversation_id) REFERENCES companion_conversation (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='消息表';

-- 记忆表，保存从用户对话中抽取并用于上下文召回的记忆
CREATE TABLE companion_memory (
    memory_id VARCHAR(64) NOT NULL PRIMARY KEY COMMENT '记忆唯一标识，由服务端生成，通常使用 mem_ 前缀',
    user_id VARCHAR(64) NOT NULL COMMENT '所属用户唯一标识；记忆只能在该用户的上下文中召回',
    layer VARCHAR(16) NOT NULL COMMENT '记忆层级；当前使用 L1，表示从单条消息中抽取的短期/事实记忆',
    kind VARCHAR(32) NOT NULL COMMENT '记忆类型：preference 表示偏好，fact 表示事实，goal 表示目标',
    content TEXT NOT NULL COMMENT '记忆内容；不得保存密码、令牌、银行卡等敏感信息',
    source_message_id VARCHAR(64) NOT NULL COMMENT '产生该记忆的源消息唯一标识，用于反馈纠正、忘记和生命周期追踪',
    confidence DECIMAL(5,4) NOT NULL COMMENT '模型对记忆内容正确性的置信度，取值范围为 0.0000 至 1.0000',
    importance INT NOT NULL COMMENT '记忆重要性评分，当前使用 1 至 5 的整数；数值越大越优先召回',
    status VARCHAR(16) NOT NULL COMMENT '记忆状态：active 表示可召回，deleted 表示已删除且不可召回',
    created_at DATETIME(6) NOT NULL COMMENT '记忆首次创建时间，使用 UTC 时间，精确到微秒',
    updated_at DATETIME(6) NOT NULL COMMENT '记忆最后更新时间，使用 UTC 时间，精确到微秒',
    -- 按用户、状态和重要性优先筛选可召回记忆，再按更新时间排序。
    KEY idx_companion_memory_user_status (user_id, status, importance, updated_at),
    -- 根据源消息定位并更新该消息产生的记忆。
    KEY idx_companion_memory_source (source_message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='记忆表';
