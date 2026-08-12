# companion-service

第一版 AI 伴侣应用服务，负责会话和 Companion 领域逻辑，不直接访问模型供应商。

## 当前能力

- 自动恢复当前用户与 Companion 的连续聊天上下文
- 加载聊天时间线和历史消息
- 保存用户消息和助手消息
- 支持非流式和 SSE/gRPC 流式回复
- 异步抽取并召回低风险 L1 记忆
- 使用 PostgreSQL + pgvector 做语义记忆召回
- 支持音频消息 STT -> Chat -> TTS 闭环
- 支持图片和视频消息，媒体文件由 `asset-service` 管理并以多模态内容部件发送给模型
- debug 和生产环境的记忆任务默认通过 RocketMQ 投递
- 支持按消息执行忘记、停止记忆和纠正反馈，不要求用户管理会话 ID
- 对明确危机表达返回固定安全转介响应
- 通过 gRPC 调用 `model-gateway` 的 Chat、Embedding、STT 和 TTS
- PostgreSQL 持久化会话、消息、L1 记忆和向量
- Kratos HTTP/gRPC 双服务
- Proto API 契约和 Wire 装配

L2/L3 分层记忆、视频关键帧/OCR/音频摘要、音频原始对象持久化和流式 TTS 属于后续增强；当前媒体上传和音频接口已经支持端到端同步测试。

## 本地运行

`companion-service` 使用 PostgreSQL + pgvector，`model-gateway` 不使用数据库。请先阅读 [`docs/database.md`](docs/database.md) 创建数据库并执行 [`docs/sql/companion-service.sql`](docs/sql/companion-service.sql)，再启动两个服务。

```bash
# 这个 Key 必须和 model-gateway 进程使用的值完全一致。
export MODEL_GATEWAY_INBOUND_API_KEY='companion-dev-internal-key'
export COMPANION_DATABASE_DSN='postgres://gaoyong@127.0.0.1:5432/companion-service?sslmode=disable'
make api
make config
make wire
make build
./bin/server -mode debug
```

## 竞品
- CharacterMe: AI Chat Bot（Google Play）
- AI Girlfriend: AI Chat（App Store）
- Dream Girl: Waifu AI Chat（Google Play ）
- Meetra AI-Chat with Characters（Google Play）
- Afizzy - AI Character Chat（App Store）
- Bae: More Than Just Chat
- Rubii
- SpiChat AI
- Afizzy
