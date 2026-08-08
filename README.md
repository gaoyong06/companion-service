# companion-service

第一版 AI 伴侣应用服务，负责会话和 Companion 领域逻辑，不直接访问模型供应商。

## 当前能力

- 创建会话
- 查询当前用户的会话列表
- 查询会话及最近消息
- 关闭会话并生成摘要
- 保存用户消息和助手消息
- 支持非流式和 SSE/gRPC 流式回复
- 异步抽取并召回低风险 L1 记忆
- 支持按消息执行忘记、停止记忆和纠正反馈
- 支持账号级数据导出和删除
- 对明确危机表达返回固定安全转介响应
- 通过 gRPC 调用 `model-gateway` 的 Chat Completion
- MySQL 持久化会话、消息和 L1 记忆
- Kratos HTTP/gRPC 双服务
- Proto API 契约和 Wire 装配

L2/L3 分层记忆、向量召回、语音输入和语音播放将在当前文本链路稳定后增加。

## 本地运行

`companion-service` 当前只使用 MySQL，`model-gateway` 不使用数据库。请先阅读 [`docs/database.md`](docs/database.md) 创建 `companion-service` 数据库并执行 [`docs/sql/companion-service.sql`](docs/sql/companion-service.sql)，再启动 `model-gateway`。

```bash
# 这个 Key 必须和 model-gateway 进程使用的值完全一致。
export MODEL_GATEWAY_INBOUND_API_KEY='companion-dev-internal-key'
export COMPANION_DATABASE_DSN='root:root@tcp(127.0.0.1:3306)/companion-service?charset=utf8mb4&parseTime=True&loc=Local'
make api
make config
make wire
make build
./bin/server -mode debug
```
