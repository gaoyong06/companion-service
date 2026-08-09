# Companion 核心 UML 图

> 文档状态：Draft  
> 文档版本：0.1  
> 对应文档：`design.md`、`tech.md`  
> 更新日期：2026-08-09

本文档只保留核心状态、时序和流程图。节点名称与技术文档中的领域模型保持一致，具体字段以 `tech.md` 为准。

## 1. 会话状态图

```mermaid
stateDiagram-v2
    [*] --> Created
    Created --> Active: first message
    Active --> Waiting: response completed
    Waiting --> Active: new message
    Active --> Interrupted: user interrupts
    Interrupted --> Active: retry or new message
    Waiting --> Segmenting: context budget reached
    Segmenting --> Active: summary persisted and new segment selected
    Active --> Archived: retention policy
    Active --> Failed: provider or policy failure
    Failed --> Active: retry
    Failed --> Archived: abandon
    Archived --> [*]
```

## 2. 破冰阶段状态图

```mermaid
stateDiagram-v2
    [*] --> first_meeting: 创建首个内部上下文
    first_meeting --> small_talk: 成功完成首次介绍回复
    small_talk --> getting_to_know: 成功完成闲聊回复
    getting_to_know --> trust: 成功完成追问回复
    trust --> established: 成功说明记忆边界并回复
    established --> established: 后续自然陪伴
    first_meeting --> first_meeting: 模型失败或危机拦截
    small_talk --> small_talk: 模型失败或危机拦截
    getting_to_know --> getting_to_know: 模型失败或危机拦截
    trust --> trust: 模型失败或危机拦截
```

阶段字段只由 `companion-service` 内部维护，不进入 HTTP/gRPC 响应；只有模型回复生成并持久化成功后才推进，危机安全回复不推进阶段。

阶段与用户体验的对应关系：`first_meeting` 负责互报名字，`small_talk` 负责探测状态，`getting_to_know` 负责基于上一条回答追问一个具体点，`trust` 负责自然说明记忆边界，`established` 进入普通陪伴。用户看不到状态字段，也不会被要求创建或切换会话。

## 3. 记忆生命周期状态图

```mermaid
stateDiagram-v2
    [*] --> Candidate: extracted from L0
    Candidate --> Rejected: consent denied or sensitive
    Candidate --> Active: policy gate passed
    Candidate --> NeedsConfirmation: conflict or low confidence
    NeedsConfirmation --> Active: user confirms
    NeedsConfirmation --> Rejected: user rejects
    Active --> Paused: user pauses recall
    Paused --> Active: user resumes recall
    Active --> Expired: expiration reached
    Active --> NeedsConfirmation: conflicting fact found
    Active --> Deleted: retention or operations cleanup
    Paused --> Deleted: retention or operations cleanup
    Expired --> Deleted: retention cleanup
    Rejected --> Deleted: cleanup completed
    Deleted --> [*]
```

## 4. 文本对话时序图

```mermaid
sequenceDiagram
    autonumber
    actor User as 用户
    participant API as Conversation API
    participant Orchestrator as Conversation Orchestrator
    participant Safety as Safety Policy
    participant Context as Context Builder
    participant Recall as Recall Engine
    participant Model as Model Gateway
    participant Store as Conversation Store
    participant Jobs as Job Queue

    User->>API: 发送消息
    API->>Orchestrator: 校验身份并恢复当前上下文
    Orchestrator->>Store: 获取或创建活动上下文
    Orchestrator->>Store: 写入 user message
    Orchestrator->>Safety: 检查输入和会话状态
    Safety-->>Orchestrator: allow / safe response policy
    Orchestrator->>Context: 构建 PromptContext
    Context->>Store: 读取摘要和最近消息
    Context->>Recall: 查询相关记忆
    Recall-->>Context: 返回有限记忆集合
    Context-->>Orchestrator: 返回上下文
    Orchestrator->>Model: 发起流式生成
    loop streaming
        Model-->>API: message.delta
        API-->>User: 展示增量文本
    end
    Model-->>Orchestrator: message.completed
    Orchestrator->>Store: 写入 assistant message
    Orchestrator->>Store: 推进破冰阶段（仅成功回复）
    Orchestrator->>Jobs: 发布 extract_memory
    Orchestrator-->>API: 返回完成状态
```

## 5. 自然破冰对话流程图

```mermaid
flowchart TD
    Start[用户直接发送消息] --> Resolve[恢复或创建活动上下文]
    Resolve --> Stage{读取 onboarding_stage}
    Stage -->|first_meeting| First[介绍固定名字并询问称呼]
    Stage -->|small_talk| Small[回应当下并询问最近状态]
    Stage -->|getting_to_know| Know[从上一条回答挑一个点追问]
    Stage -->|trust| Trust[回应当下并说明记忆边界]
    Stage -->|established| Established[自然陪伴，最多一个问题]
    First --> Generate[模型生成非空回复]
    Small --> Generate
    Know --> Generate
    Trust --> Generate
    Established --> Generate
    Generate --> Persist[持久化 assistant message]
    Persist --> Advance[推进到下一个内部阶段]
    Advance --> Continue[返回回复并继续对话]
    Generate -->|模型失败或空回复| Hold[保持当前阶段并返回错误]
    Stage -->|危机输入| Safety[返回安全转介回复]
    Safety --> HoldSafety[保持当前阶段]
```

## 6. 自然破冰时序图

```mermaid
sequenceDiagram
    autonumber
    actor User as 用户
    participant API as Conversation API
    participant Store as Conversation Store
    participant Context as Context Builder
    participant Lexicon as Lexicon Catalog
    participant Model as Model Gateway

    User->>API: 直接发送第一条消息
    API->>Store: 获取或创建活动上下文
    Store-->>API: first_meeting
    API->>Context: 构建当前阶段上下文
    Context->>Lexicon: 读取 first_meeting 词条
    Lexicon-->>Context: 固定介绍与称呼引导
    Context->>Model: 发送通用提示词 + 阶段词条 + 用户输入
    Model-->>API: 简短回复
    API->>Store: 持久化 assistant message
    API->>Store: first_meeting -> small_talk
    API-->>User: 返回回复

    User->>API: 继续自然回复
    API->>Store: 读取当前阶段和连续历史
    Store-->>API: small_talk / getting_to_know / trust
    API->>Context: 选择对应阶段词条
    Context->>Model: 发送阶段化上下文
    Model-->>API: 一到三句回复
    API->>Store: 持久化回复并推进阶段
    API-->>User: 返回回复
```

## 7. 记忆抽取与写入时序图

```mermaid
sequenceDiagram
    autonumber
    participant Queue as RocketMQ
    participant Extractor as Candidate Extractor
    participant Classifier as Sensitivity Classifier
    participant Dedup as Deduplicator
    participant Conflict as Conflict Detector
    participant Consent as Consent Gate
    participant Memory as Memory Store
    participant Vector as Vector Store
    participant User as 用户

    Queue->>Extractor: extract_memory(message_id)
    Extractor->>Classifier: 提交候选记忆
    Classifier-->>Extractor: sensitivity and confidence
    Extractor->>Dedup: 规范化并去重
    Dedup->>Conflict: 对比现有事实
    Conflict-->>Dedup: no conflict / conflict
    Dedup->>Consent: 检查用户记忆授权
    alt 自动写入
        Consent-->>Memory: activate L1 memory
        Memory->>Vector: 写入 embedding
    else 需要确认
        Consent-->>Memory: save candidate
        Memory->>User: 在自然对话中请求确认
    else 拒绝保存
        Consent-->>Memory: mark rejected
    end
```

## 8. 上下文召回流程图

```mermaid
flowchart LR
    Input[用户输入] --> Budget[建立上下文预算]
    Budget --> Recent[读取最近消息]
    Budget --> Summary[读取内部上下文摘要]
    Budget --> Profile[读取精简 Persona]
    Budget --> Relation[读取关系状态]
    Input --> Keyword[关键词检索]
    Input --> Vector[向量检索]
    Keyword --> Fusion[混合排序与去重]
    Vector --> Fusion
    Fusion --> Policy[敏感性与冲突策略]
    Policy --> Limit[数量和字符数裁剪]
    Recent --> Prompt[PromptContext]
    Summary --> Prompt
    Profile --> Prompt
    Relation --> Prompt
    Limit --> Prompt
    Prompt --> Model[Model Gateway]
    Model --> Stream[流式回复]

    Budget -->|超过滚动阈值| Summarize[生成内部摘要]
    Summarize --> Roll[关闭旧上下文并创建新上下文]
    Roll --> Prompt
```

## 9. 图片和视频消息时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Web as companion-web
    participant API as companion-service
    participant Asset as asset-service
    participant Gateway as model-gateway
    participant Provider as 多模态模型

    User->>Web: 选择图片或视频并发送
    Web->>API: multipart /companion/v1/media-messages
    API->>Asset: UploadFile(data, metadata)
    Asset-->>API: asset_id + url
    API->>API: 写入消息和资产关联
    API->>Gateway: Chat(content_parts)
    Gateway->>Provider: image_url / video_url
    Provider-->>Gateway: 回复
    Gateway-->>API: 统一文本回复
    API-->>Web: 用户媒体消息和 Companion 回复
```

## 10. 运营侧账号级数据治理流程图

```mermaid
flowchart TD
    Request[运营系统提交治理任务] --> Verify[校验运营授权和目标用户]
    Verify --> Mark[标记 user deletion pending]
    Mark --> Conversation[删除会话和消息]
    Mark --> Memory[删除 L1/L2/L3/R 记忆]
    Mark --> Vector[删除向量索引]
    Mark --> Cache[清理缓存]
    Mark --> Object[删除音频和大文本]
    Mark --> Queue[取消或过期异步任务]
    Conversation --> Check[删除结果校验]
    Memory --> Check
    Vector --> Check
    Cache --> Check
    Object --> Check
    Queue --> Check
    Check -->|全部完成| Completed[delete completed]
    Check -->|存在失败| Retry[重试并告警]
    Retry --> Check
```

## 11. 语音交互时序图

```mermaid
sequenceDiagram
    autonumber
    actor User as 用户
    participant App as Web/Mobile App
    participant Companion as Companion API
    participant Model as Model Gateway
    participant STT as STT Provider
    participant TTS as TTS Provider

    User->>App: 按住说话
    App->>Companion: SendAudioMessage(audio_data)
    User->>App: 松开
    Companion->>Model: TranscribeAudio
    Model->>STT: POST /v1/audio/transcriptions
    STT-->>Model: transcription
    Companion->>Model: ChatCompletion(transcription)
    Model-->>Companion: assistant text
    Companion->>Model: SynthesizeSpeech(assistant text)
    Model->>TTS: POST /v1/audio/speech
    TTS-->>Model: audio bytes
    Companion-->>App: text messages + audio bytes
    App-->>User: 播放语音
```

## 12. 图之间的关系

```text
会话状态图：定义一次会话的生命周期
破冰阶段状态图：定义首次社交过程的服务端节奏
记忆状态图：定义一条记忆的治理生命周期
对话时序图：定义一次文本请求的在线链路
记忆时序图：定义离线记忆写入和用户确认链路
召回流程图：定义 PromptContext 的组装方式
运营治理流程图：定义内部账号级数据删除的闭环，用户侧不暴露对应 API
语音时序图：定义 MVP 的语音输入和播放链路
```
