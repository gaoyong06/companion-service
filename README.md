# companion-service

第一版 AI 伴侣应用服务，负责会话和 Companion 领域逻辑，不直接访问模型供应商。

## 当前能力

- 创建会话
- 查询会话及最近消息
- 保存用户消息和助手消息
- 通过 gRPC 调用 `model-gateway` 的 Chat Completion
- MySQL 持久化会话和消息
- Kratos HTTP/gRPC 双服务
- Proto API 契约和 Wire 装配

记忆抽取、L1-L3 分层记忆、语音输入和语音播放将在当前会话链路稳定后增加。

## 本地运行

先创建数据库并执行 `docs/sql/companion-service.sql`，再启动 `model-gateway`。

```bash
export MODEL_GATEWAY_API_KEY=your-provider-api-key
export MODEL_GATEWAY_INBOUND_API_KEY=shared-internal-key
export COMPANION_DATABASE_DSN='root:root@tcp(127.0.0.1:3306)/companion-service?charset=utf8mb4&parseTime=True&loc=Local'
make api
make config
make wire
make build
./bin/server -mode debug
```
