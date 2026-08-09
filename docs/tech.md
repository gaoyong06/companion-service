# Companion 技术设计文档

> 文档状态：Draft  
> 文档版本：0.1  
> 服务名称：`companion-service`  
> 对应产品文档：`docs/design.md`  
> 更新日期：2026-08-08

## 1. 技术目标

`companion-service` 是个人 AI 伙伴的应用服务，负责会话编排、上下文构建、记忆治理、人格策略、关系状态、安全策略和模型供应商适配。

技术设计优先保证：

1. 当前会话低延迟和流式输出。
2. 长期记忆可追溯、可纠正、可删除。
3. 模型、Embedding、STT、TTS 供应商可替换。
4. 用户数据隔离和删除链路完整。
5. MVP 内部模块化，后续可以按边界拆分服务。

## 2. 非目标

- 不在本服务内实现通用大模型训练。
- 不把业务逻辑绑定到某一家 LLM 或向量数据库。
- 不在第一版引入多 Agent 协作和复杂工作流引擎。
- 不在业务请求线程中同步完成全部记忆抽取和 Persona 重建。

## 3. 总体架构

### 3.1 逻辑模块

```text
API Layer
  ├── Conversation API
  ├── Memory API
  ├── Profile API
  ├── Consent API
  └── Data Export/Delete API

Application Layer
  ├── Conversation Orchestrator
  ├── Context Builder
  ├── Recall Policy
  ├── Memory Governance
  ├── Relationship Policy
  ├── Safety Policy
  └── Voice Orchestrator

Domain Layer
  ├── Conversation
  ├── Memory
  ├── Scenario
  ├── Persona
  ├── Relationship
  ├── Consent
  └── Safety Incident

Infrastructure Layer
  ├── Relational Repository
  ├── Vector Repository
  ├── Object Storage
  ├── Event Bus / Job Queue
  ├── Model Gateway
  └── Observability
```

### 3.2 推荐部署形态

产品 MVP 使用两个 Go 服务部署：`companion-service` 负责会话编排和 Companion 领域逻辑，`model-gateway` 负责统一访问模型供应商。两者通过 gRPC 通信，供应商密钥只保存在 `model-gateway`。

当前 0.1 实现边界如下：

- `companion-service` 已实现创建会话、文本/音频消息、流式回复、语义记忆召回和数据治理。
- `model-gateway` 已实现 OpenAI-compatible Chat、Embedding、STT、TTS 和模型列表。
- PostgreSQL + pgvector 持久化会话、消息、L1 记忆及其向量；生产记忆任务通过 RocketMQ，debug 配置可显式关闭队列使用进程内处理。
- 供应商 API Key 只保留在 `model-gateway`，业务服务只持有内部网关 Key。

生产环境的 `queue.driver=rocketmq` 使用 `companion_memory_extract` topic 和独立 producer/consumer group；消费失败由 RocketMQ 重试并进入该消费组的 DLQ。debug 配置将 `queue.enabled` 设为 `false`，只用于本地验证，不代表生产可以省略 RocketMQ。

后续可以按负载和数据隔离需求拆分：

```text
companion-service
  -> companion-memory-service
  -> companion-voice-service
  -> companion-notification-service
```

拆分前必须先证明独立扩缩容、独立权限或独立数据生命周期确实存在。

## 4. 核心运行时流程

### 4.1 文本对话请求

1. API 层校验用户身份、会话权限和请求大小。
2. Conversation Orchestrator 写入用户消息事件。
3. Safety Policy 对当前输入做快速检查。
4. Context Builder 读取最近消息、会话摘要、精简 Persona、关系状态和召回结果。
5. Context Builder 始终注入 Companion system prompt：首次对话执行简短认识流程；后续对话遵循陪伴优先、默认短回复、单问题和不主动建议规则。
6. Model Gateway 发起模型请求；非流式和流式两种链路均可用，普通陪伴回复默认设置 `max_tokens=256`。
7. 服务写入 Assistant 消息并返回完整回复，流式链路通过 SSE 或 gRPC 转发 token。
8. 后续异步任务判断是否需要 L1 抽取、L2 场景聚合和 Persona 更新。

### 4.2 语音请求

```text
Audio Upload
  -> Speech-to-Text
  -> Conversation Orchestrator
  -> Context Builder
  -> Streaming LLM
  -> Text-to-Speech
  -> Audio Stream
```

STT、LLM 和 TTS 均通过 Model Gateway 访问。业务模块不得直接依赖供应商 SDK。

## 5. 领域模型

领域实体不使用含义不明确的裸字段 `id`。所有由 `companion-service` 生成和管理的标识，都使用 `<实体名>_id` 命名，并在服务内保持唯一；关联字段必须明确表达被关联的实体，例如 `conversation_id`、`message_id` 和 `memory_id`。

### 5.1 Conversation

当前实现对应 `internal/data.ConversationModel` 和 `companion_conversation` 表。

```text
Conversation
- conversation_id: 会话唯一标识，由 companion-service 生成，通常使用 conv_ 前缀。
- user_id: 所属用户唯一标识，用于数据隔离、权限校验和用户级删除。
- companion_id: 陪伴角色唯一标识，当前默认值为 default。
- status: 会话状态；当前使用 active 和 closed，未来可扩展 archived、deleting。
- summary: 会话摘要；关闭会话时由模型生成，用于保留长期上下文和未解决事项。
- created_at: 会话创建时间，使用 UTC 时间。
- updated_at: 会话最后更新时间；新消息写入或会话关闭时更新。
```

以下字段属于后续扩展设计，当前未在 Go 模型和 SQL 中实现：

```text
- summary_version: 摘要版本号，用于摘要重建和并发更新控制。
- last_message_at: 最后一条消息的业务时间；当前使用 updated_at 代替。
- closed_at: 会话实际关闭时间；当前未单独保存。
```

### 5.2 Message

当前实现对应 `internal/data.MessageModel` 和 `companion_message` 表。

```text
Message
- message_id: 消息唯一标识，由 companion-service 生成，通常使用 msg_ 前缀。
- conversation_id: 所属会话唯一标识，必须关联已存在的 Conversation。
- user_id: 所属用户唯一标识；冗余保存以支持用户范围查询和删除。
- role: 消息角色；user 表示用户消息，assistant 表示陪伴回复，system 仅用于模型上下文。
- content: 消息正文，保存用户输入或模型生成的 UTF-8 文本。
- created_at: 消息创建时间，使用 UTC 时间。
```

以下字段属于后续语音和可观测性设计，当前未在 Go 模型和 SQL 中实现：

```text
- modality: 消息载体类型；计划支持 text、audio、transcription。
- content_ref: 对象存储引用，用于保存原始音频或超长内容。
- content_hash: 内容完整性校验值，用于去重和审计。
- token_count: 本条消息对应的模型 token 数量，用于成本和上下文统计。
- safety_level: 消息安全等级，用于安全策略和审计。
```

### 5.3 Memory

当前实现对应 `internal/data.MemoryModel` 和 `companion_memory` 表。

```text
Memory
- memory_id: 记忆唯一标识，由 companion-service 生成，通常使用 mem_ 前缀。
- user_id: 所属用户唯一标识；记忆只能在该用户的上下文中召回。
- layer: 记忆层级；当前使用 L1，表示从单条消息中抽取的原子记忆。
- kind: 记忆类型；当前支持 preference、fact、goal。
- content: 记忆正文；不得保存密码、令牌、银行卡等敏感信息。
- source_message_id: 产生该记忆的源消息唯一标识，用于反馈纠正、忘记和生命周期追踪。
- confidence: 模型对记忆内容正确性的置信度，范围为 0.0 至 1.0。
- importance: 记忆重要性评分，当前为 1 至 5 的整数，数值越大越优先召回。
- status: 记忆状态；当前使用 active 和 deleted。
- created_at: 记忆首次创建时间，使用 UTC 时间。
- updated_at: 记忆最后更新时间，使用 UTC 时间。
```

以下字段属于分层记忆的后续设计，当前未在 Go 模型和 SQL 中实现：

```text
- normalized_content: 规范化后的记忆正文，用于去重和冲突判断。
- sensitivity: 敏感级别，用于决定是否需要用户确认。
- source_message_ids: 多条来源消息标识；当前使用单个 source_message_id。
- source_scenario_ids: 来源场景记忆标识；当前没有 Scenario 模型。
- first_seen_at: 首次观察时间；当前使用 created_at 代替。
- last_confirmed_at: 最近确认时间；当前使用 updated_at 代替。
- expires_at: 记忆过期时间；当前没有自动过期任务。
- version: 记忆版本号；当前通过 updated_at 更新同一条记录。
```

Memory 长期演进时必须成为版本化实体；当前实现仍使用单条记录更新，不能将当前实现误认为已具备完整版本审计能力。

### 5.4 Consent

Consent 当前只有产品和技术设计，没有对应的 Go 模型、SQL 表或 API。

```text
Consent
- consent_id: 用户授权记录唯一标识，未来由 companion-service 生成。
- user_id: 授权所属用户唯一标识。
- memory_mode: 长期记忆模式；cautious 表示谨慎记忆，normal 表示正常记忆，disabled 表示关闭。
- allow_sensitive_memory: 是否允许保存需要确认的敏感记忆。
- allow_proactive_contact: 是否允许主动联系用户。
- proactive_time_window: 允许主动联系的时间窗口。
- allow_voice_storage: 是否允许保存原始语音。
- updated_at: 授权配置最后更新时间，使用 UTC 时间。
```

Consent 变更必须产生事件，并影响后续写入、召回和异步任务。

### 5.5 RelationshipState

RelationshipState 当前只有产品和技术设计，没有对应的 Go 模型、SQL 表或 API。

```text
RelationshipState
- relationship_state_id: 关系状态记录唯一标识，未来由 companion-service 生成。
- user_id: 关系状态所属用户唯一标识。
- stage: 关系阶段；new 表示初次认识，familiar 表示熟悉，established 表示稳定互动。
- preferred_tone: 用户偏好的交流语气。
- preferred_address: Companion 对用户的称呼。
- current_mode: 当前互动模式；listening、advice、planning 或 casual。
- recent_emotional_context: 最近一次确认的情绪上下文摘要。
- unresolved_topics: 尚未解决或待跟进的话题摘要。
- last_confirmed_at: 用户最近确认关系状态的时间，使用 UTC 时间。
- version: 关系状态版本号，用于并发更新控制。
```

RelationshipState 只表达可观察和可配置的交互策略，不表达 AI 的主观情感或占有关系。

## 6. 数据存储

### 6.1 当前 0.1 实现

| 数据 | 存储 | 原因 |
| --- | --- | --- |
| 会话、消息、L1 记忆和向量 | PostgreSQL + pgvector | 当前代码使用 GORM PostgreSQL 驱动；记忆向量使用 1536 维 cosine 检索 |
| 模型调用配置 | 环境变量和 YAML | `model-gateway` 当前无状态，不落库 |
| Embedding | `companion_memory.embedding` | 通过 `model-gateway` 生成并在 PostgreSQL 中做向量召回 |
| STT、TTS 音频 | 请求生命周期内传输 | 供应商调用通过 `model-gateway` 转发；原始音频持久化仍由后续 `asset-service` 调用方决定 |

TencentDB-Agent-Memory 的 SQLite + sqlite-vec 方案适合本地原型或单机部署。生产环境应保留 Repository 接口，避免业务层绑定具体存储实现。

### 6.2 当前数据库和建库入口

- 当前只需要创建一个 PostgreSQL 数据库：`companion_service`，并启用 `vector` 扩展。
- `model-gateway` 不需要创建数据库。
- 建库、连接参数和手动执行方式见 `docs/database.md`。
- 当前 SQL 创建 `companion_conversation`、`companion_message` 和 `companion_memory` 三张表，其中记忆表包含 1536 维向量列和 HNSW 索引。

### 6.3 未进入当前版本的演进方案

以下方案属于后续能力的候选设计，不能作为当前部署前置条件：

| 数据 | 候选存储 | 触发条件 |
| --- | --- | --- |
| L2/L3/R 版本化记忆 | PostgreSQL JSONB + 普通列 | 分层记忆和版本审计进入开发 |
| 原始音频和大文本 | 对象存储 | 语音和大内容持久化进入开发 |
| 热门会话状态 | Redis，可选 | 监测证明缓存能降低实际延迟 |

当前已经创建并使用 PostgreSQL、pgvector；生产配置使用 RocketMQ。Redis 和 OSS 只有在对应持久化调用方合入后才增加资源。

### 6.4 当前索引

- `companion_conversation(user_id, created_at)`
- `companion_message(conversation_id, created_at)`
- `companion_message(user_id, created_at)`
- `companion_memory(user_id, status, importance, updated_at)`
- `companion_memory(source_message_id)`
- `companion_memory.embedding` 的 HNSW cosine 索引

`memory_sources` 表和按层级的 PostgreSQL 索引属于未来设计，当前不存在。

## 7. 记忆流水线

### 7.1 写入

```text
MessageCommitted
  -> CandidateExtractor
  -> SensitivityClassifier
  -> Deduplicator
  -> ConflictDetector
  -> ConsentGate
  -> MemoryWriter
  -> EmbeddingWriter
```

CandidateExtractor 只能产生候选记忆。最终是否激活由 `ConsentGate` 和规则策略决定。

### 7.2 分层生成

- L0：每条消息持久化，并建立来源关系。
- L1：按事件或每 N 轮异步抽取原子事实。
- L2：同一用户的相关 L1 聚合为场景，记录状态、进展和时间范围。
- L3：从稳定且已确认的 L1/L2 生成 Persona 片段。
- R：根据最近互动和用户设置更新关系状态，不从单轮情绪直接推断长期关系。

### 7.3 冲突处理

例如旧记忆为“用户住在上海”，新消息为“我已经搬到杭州”。系统应：

1. 保留旧记忆来源，不直接删除证据。
2. 创建新候选事实。
3. 将旧事实标记为 `needs_confirmation` 或设置结束时间。
4. 在下一次合适的对话中询问或使用较新的事实。

## 8. 上下文构建

Context Builder 生成有限预算的 `PromptContext`：

```text
PromptContext
- system_policy
- companion_persona
- relationship_summary
- current_conversation_summary
- recent_messages
- recalled_memories
- user_input
- output_constraints
```

建议默认预算：

| 区域 | 预算策略 |
| --- | --- |
| 系统和安全策略 | 固定，不可被用户输入覆盖 |
| Persona 和关系摘要 | 小而稳定，超限时不扩张 |
| 最近消息 | 优先保留当前会话连续性 |
| 长期记忆 | 最多 5 条 L1 + 1 条 L2 |
| 用户输入 | 永远完整保留 |

召回采用关键词、向量和规则的混合策略。单次召回必须设置总字符数和超时，超时则跳过长期记忆，不阻塞当前回复。

当前 0.1 已接入受控 L1 记忆召回，Context Builder 对记忆和最近消息共设置 24 KiB 总字符预算，始终保留当前输入，并从最新历史向前截取可用消息。

消息进入模型前会经过基础危机词检测。命中明确的自伤或轻生表达时，服务保存用户消息并返回固定安全转介文本，不调用模型，也不提交记忆抽取任务；这不是医疗或危机干预服务的替代品。

## 9. API 草案

### 9.1 对话

```http
POST /companion/v1/conversations
GET  /companion/v1/conversations
POST /companion/v1/conversations/{conversation_id}/messages
POST /companion/v1/conversations/{conversation_id}/messages:stream
GET  /companion/v1/conversations/{conversation_id}
POST /companion/v1/conversations/{conversation_id}/close
```

消息请求示例：

```json
{
  "client_message_id": "cmsg_01J...",
  "modality": "text",
  "content": "我今天有点累，只想聊一会儿",
  "remember": "default"
}
```

当前 SSE 返回 `data: MessageChunk` JSON；事件封装和 WebSocket 属于后续协议演进。

规划中的事件类型包括：

```text
message.started
message.delta
message.completed
message.interrupted
message.failed
```

### 9.2 轻量记忆反馈

```http
POST /companion/v1/conversations/{conversation_id}/memory-feedback
```

请求示例：

```json
{
  "conversation_id": "conv_01J...",
  "message_id": "msg_01J...",
  "action": "forget",
  "kind": "fact",
  "content": "用户已经搬到杭州"
}
```

`action` 支持 `correct`、`forget` 和 `do_not_remember`。`correct` 会先撤销来源消息产生的记忆，再写入一条经过字段校验的 L1 记忆；普通用户不会获得按记忆 ID 的 CRUD。

MVP 不向普通用户暴露按记忆 ID 浏览和修改的完整 CRUD。服务内部仍保留版本化 Memory Store，便于冲突处理、删除传播、质量评估和后台审计。

### 9.3 用户控制与数据

```http
GET   /api/v1/consents
PATCH /api/v1/consents
POST  /companion/v1/data/export
POST  /companion/v1/data/delete
```

### 9.4 错误契约

错误信息和日志使用英文，返回结构保持稳定：

```json
{
  "code": "MEMORY_RECALL_TIMEOUT",
  "message": "Memory recall timed out",
  "request_id": "req_01J..."
}
```

## 10. 一致性与幂等

- 客户端使用 `client_message_id` 防止网络重试产生重复消息。
- Message 写入和 `MessageCommitted` 事件在同一事务中完成。
- 记忆抽取任务以 `message_id` 或 `conversation_summary_version` 做幂等键。
- 删除任务必须可重试，重复执行不得恢复已删除数据。
- 流式回复中断时，保存已生成内容和 `interrupted` 状态，不把未发送文本当作完整回复。

## 11. 安全与隐私实现

### 11.1 权限

- 所有资源查询必须同时校验 `user_id` 和资源归属。
- 管理接口与用户接口分离。
- 服务间调用使用服务身份，不透传用户 Token 作为唯一授权依据。

### 11.2 删除传播

```text
DeleteRequested
  -> ConversationStore 删除
  -> MemoryStore 删除
  -> VectorStore 删除
  -> Cache 删除
  -> ObjectStore 删除
  -> Queue 中的任务取消或标记过期
  -> DeleteCompleted
```

删除链路需要有可查询的任务状态和审计记录，但审计记录不得保留被删除的原始内容。运营后台只能管理策略、状态和聚合指标；访问原始内容必须有明确授权、脱敏和审计。

### 11.3 供应商隔离

定义统一接口：

```text
ChatModel
EmbeddingModel
SpeechToText
TextToSpeech
```

Provider Adapter 负责请求格式、重试、超时、计费信息和敏感字段过滤。模型供应商不可直接访问数据库。

## 12. 异步任务与失败策略

异步任务包括：

- `extract_memory`
- `build_scenario`
- `refresh_persona`
- `generate_embedding`
- `propagate_delete`
- `generate_data_export`

失败策略：

1. 记忆任务失败不影响当前对话回复。
2. 召回超时直接跳过，并记录英文告警日志。
3. 模型超时返回可识别的降级错误，不伪造成功回复。
4. 删除任务失败必须持续重试并告警。
5. 所有任务使用指数退避和最大重试次数，超过后进入死信队列。

## 13. 可观测性

每次请求携带 `request_id`，每个异步任务携带 `job_id`。指标至少包括：

- `conversation_first_token_latency_ms`
- `conversation_stream_duration_ms`
- `memory_recall_latency_ms`
- `memory_recall_timeout_total`
- `memory_candidate_total`
- `memory_conflict_total`
- `memory_delete_residual_total`
- `provider_error_total{provider,operation}`
- `safety_escalation_total`

日志必须默认脱敏。禁止输出完整用户输入、完整模型回复、API Key 和原始音频地址。

## 14. 性能目标

| 指标 | MVP 目标 |
| --- | --- |
| 文本首 token P95 | <= 2 秒，不含供应商不可控排队 |
| 记忆召回 P95 | <= 300ms |
| 记忆召回超时 | 5 秒后跳过，不阻塞回复 |
| 语音开始播放 P95 | <= 3 秒 |
| 单用户并发生成 | 默认 1 个，后续可配置 |
| 删除任务完成 | 24 小时内完成并可查询 |

## 15. 测试策略

### 15.1 单元测试

- 召回排序和预算裁剪。
- 记忆去重和冲突检测。
- ConsentGate 敏感信息决策。
- 删除任务幂等。
- PromptContext 序列化。

### 15.2 集成测试

- 完整对话链路：消息、流式回复、异步记忆、再次召回。
- 供应商超时、限流和错误码映射。
- 用户删除后多存储删除传播。
- 多用户数据隔离。

### 15.3 评测集

建立带来源的对话评测集，覆盖：

- 稳定偏好记忆。
- 时间变化事实。
- 相互矛盾事实。
- 明确“不允许记忆”。
- 不相关历史干扰。
- 敏感信息和安全风险。

指标不能只测回答相似度，必须单独测错误记忆和误召回。

## 16. 依赖与替换原则

TencentDB-Agent-Memory 可作为分层记忆和混合召回的参考实现。接入时只能通过 `MemoryRepository`、`RecallEngine` 和 `MemoryExtractor` 等内部接口使用，不能让业务代码依赖其文件路径、插件协议或特定 Agent 框架。

任何外部依赖都需要记录：许可证、数据是否出境、是否保留输入、超时和删除能力。
