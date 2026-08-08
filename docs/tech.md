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

- `companion-service` 已实现创建会话、查询会话、保存消息和非流式 Chat 调用。
- `model-gateway` 已实现 OpenAI-compatible Chat、Embedding 和模型列表。
- MySQL 持久化覆盖会话、消息和 L1 记忆；当前记忆使用异步候选抽取和有限召回，分层记忆、向量召回、STT 和 TTS 属于后续迭代。
- 服务内部仍按接口隔离，记忆任务已在 `companion-service` 内异步执行，再根据独立扩缩容需求拆分。

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
5. Model Gateway 发起模型请求；非流式和流式两种链路均可用。
6. 服务写入 Assistant 消息并返回完整回复，流式链路通过 SSE 或 gRPC 转发 token。
7. 后续异步任务判断是否需要 L1 抽取、L2 场景聚合和 Persona 更新。

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

### 5.1 Conversation

```text
Conversation
- id
- user_id
- companion_id
- status: active | closed | archived | deleting
- summary
- summary_version
- last_message_at
- created_at
- closed_at
```

### 5.2 Message

```text
Message
- id
- conversation_id
- user_id
- role: user | assistant | system
- modality: text | audio | transcription
- content_ref
- content_hash
- token_count
- safety_level
- created_at
```

原始音频和长文本通过 `content_ref` 指向对象存储，数据库只保存必要元数据和完整性校验值。

### 5.3 Memory

```text
Memory
- id
- user_id
- layer: L1 | L2 | L3 | R
- type
- content
- normalized_content
- confidence
- importance
- sensitivity
- status: candidate | active | rejected | expired | deleted
- source_message_ids
- source_scenario_ids
- first_seen_at
- last_confirmed_at
- expires_at
- version
```

Memory 必须是版本化实体。更新使用新版本或事件记录，不能无审计地覆盖原记录。

### 5.4 Consent

```text
Consent
- user_id
- memory_mode: cautious | normal | disabled
- allow_sensitive_memory
- allow_proactive_contact
- proactive_time_window
- allow_voice_storage
- updated_at
```

Consent 变更必须产生事件，并影响后续写入、召回和异步任务。

### 5.5 RelationshipState

```text
RelationshipState
- user_id
- stage: new | familiar | established
- preferred_tone
- preferred_address
- current_mode: listening | advice | planning | casual
- recent_emotional_context
- unresolved_topics
- last_confirmed_at
- version
```

RelationshipState 只表达可观察和可配置的交互策略，不表达 AI 的主观情感或占有关系。

## 6. 数据存储

### 6.1 当前 0.1 实现

| 数据 | 存储 | 原因 |
| --- | --- | --- |
| 会话、消息、L1 记忆 | MySQL | 当前代码使用 GORM MySQL 驱动；表结构见 `docs/sql/companion-service.sql` |
| 模型调用配置 | 环境变量和 YAML | `model-gateway` 当前无状态，不落库 |
| Embedding、STT、TTS | 当前未持久化 | 供应商调用通过 `model-gateway` 转发 |

TencentDB-Agent-Memory 的 SQLite + sqlite-vec 方案适合本地原型或单机部署。生产环境应保留 Repository 接口，避免业务层绑定具体存储实现。

### 6.2 当前数据库和建库入口

- 当前只需要创建一个 MySQL 数据库：`companion-service`。
- `model-gateway` 不需要创建数据库。
- 建库、连接参数和手动执行方式见 `docs/database.md`。
- 当前 SQL 只创建 `companion_conversation`、`companion_message` 和 `companion_memory` 三张表。

### 6.3 未来演进方案（尚未实现）

以下方案属于后续能力的候选设计，不能作为当前部署前置条件：

| 数据 | 候选存储 | 触发条件 |
| --- | --- | --- |
| L2/L3/R 版本化记忆 | PostgreSQL JSONB + 普通列 | 分层记忆和版本审计进入开发 |
| Embedding | PostgreSQL + pgvector 或独立 Vector Store | 向量召回进入开发 |
| 原始音频和大文本 | 对象存储 | 语音和大内容持久化进入开发 |
| 异步任务 | 可靠队列 | 任务量和独立重试需求超过进程内队列能力 |
| 热门会话状态 | Redis，可选 | 监测证明缓存能降低实际延迟 |

在这些能力实现之前，不创建 PostgreSQL、pgvector、Redis 或队列资源，也不提前编写没有调用方的表。

### 6.4 当前索引

- `companion_conversation(user_id, created_at)`
- `companion_message(conversation_id, created_at)`
- `companion_message(user_id, created_at)`
- `companion_memory(user_id, status, importance, updated_at)`
- `companion_memory(source_message_id)`

向量索引、`memory_sources` 表和按层级的 PostgreSQL 索引均属于未来设计，当前不存在。

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
