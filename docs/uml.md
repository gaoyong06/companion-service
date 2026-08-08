# Companion 核心 UML 图

> 文档状态：Draft  
> 文档版本：0.1  
> 对应文档：`design.md`、`tech.md`  
> 更新日期：2026-08-08

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
    Active --> Closing: close requested
    Waiting --> Closing: close requested
    Closing --> Closed: summary persisted
    Closed --> Archived: retention policy
    Closed --> Active: resume allowed
    Active --> Failed: provider or policy failure
    Failed --> Active: retry
    Failed --> Closed: abandon
    Archived --> [*]
```

## 2. 记忆生命周期状态图

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
    Active --> Deleted: user deletes
    Paused --> Deleted: user deletes
    Expired --> Deleted: retention cleanup
    Rejected --> Deleted: cleanup completed
    Deleted --> [*]
```

## 3. 文本对话时序图

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
    API->>Orchestrator: 校验身份并创建请求
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
    Orchestrator->>Jobs: 发布 extract_memory
    Orchestrator-->>API: 返回完成状态
```

## 4. 记忆抽取与写入时序图

```mermaid
sequenceDiagram
    autonumber
    participant Queue as Job Queue
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

## 5. 上下文召回流程图

```mermaid
flowchart LR
    Input[用户输入] --> Budget[建立上下文预算]
    Budget --> Recent[读取最近消息]
    Budget --> Summary[读取会话摘要]
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
```

## 6. 账号级数据删除流程图

```mermaid
flowchart TD
    Request[用户提交账号级删除请求] --> Verify[校验用户身份]
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

## 7. 语音交互时序图

```mermaid
sequenceDiagram
    autonumber
    actor User as 用户
    participant App as Mobile App
    participant Voice as Voice Orchestrator
    participant STT as Speech To Text
    participant Chat as Conversation Orchestrator
    participant Model as Model Gateway
    participant TTS as Text To Speech

    User->>App: 按住说话
    App->>Voice: 上传音频片段
    User->>App: 松开
    Voice->>STT: 转写音频
    STT-->>Voice: transcription
    Voice->>Chat: 提交 transcription
    Chat->>Model: 请求流式回复
    loop text chunks
        Model-->>Voice: text chunk
        Voice->>TTS: 合成音频片段
        TTS-->>App: audio chunk
        App-->>User: 播放语音
    end
    User->>App: 点击停止
    App->>Voice: interrupt
    Voice->>Model: cancel generation
    Voice-->>App: interrupted
```

## 8. 图之间的关系

```text
会话状态图：定义一次会话的生命周期
记忆状态图：定义一条记忆的治理生命周期
对话时序图：定义一次文本请求的在线链路
记忆时序图：定义离线记忆写入和用户确认链路
召回流程图：定义 PromptContext 的组装方式
删除流程图：定义账号级数据删除的闭环
语音时序图：定义 MVP 的语音输入和播放链路
```
