# 数据库说明

> 文档状态：Current MVP
> 更新日期：2026-08-09

## 1. 数据库清单

| 服务 | 数据库 | 当前用途 | 是否需要手动建库 |
| --- | --- | --- | --- |
| `companion-service` | PostgreSQL 15+ + pgvector | 会话、消息、L1 记忆和向量召回 | 是 |
| `model-gateway` | 无 | 无状态转发 Chat、Embedding 和模型列表请求 | 否 |

当前版本使用 PostgreSQL + pgvector；debug 和生产环境的记忆任务默认依赖 RocketMQ。媒体原始文件由 `asset-service` 的 OSS 后端保存，不需要 MySQL、Redis 或单独的向量数据库。

本地 debug 默认连接 RocketMQ NameServer `127.0.0.1:9876`，记忆 topic 为 `companion_memory_extract`，消费组为 `companion_memory_consumer`。NameServer 是 RocketMQ 的 TCP 地址，不提供 HTTP 管理页面；topic 需要由基础设施初始化脚本或 `mqadmin` 预创建。宿主机运行的服务还要求 broker 对外公布可解析的地址（本地通常为 `127.0.0.1:10911`），不能只公布 Docker 网络内的 `rocketmq-broker:10911`。

媒体上传依赖 `asset-service` gRPC（本机默认 `127.0.0.1:9104`），其 OSS/本地存储配置由 `asset-service` 自身维护；`companion-service` 不创建或迁移资产服务的数据库。

## 2. companion-service

### 2.1 数据库名称和连接

Debug 配置默认连接：

```text
postgres://gaoyong@127.0.0.1:5432/companion-service?sslmode=disable
```

本机 PostgreSQL 默认使用当前 macOS 用户 `gaoyong` 连接，不假设存在 `postgres` 超级用户，也不在 DSN 中写入密码。若本机实例启用了密码认证，请通过 `COMPANION_DATABASE_DSN` 覆盖该连接串，不要把密码提交到配置文件。

Release 配置从环境变量 `COMPANION_DATABASE_DSN` 读取完整 DSN，例如：

```bash
export COMPANION_DATABASE_DSN='postgres://companion_app:strong-password@postgres:5432/companion-service?sslmode=require'
```

连接使用 GORM PostgreSQL 驱动，代码入口为 `internal/data/conversation.go`。服务启动时不会自动创建数据库，也不会自动迁移表结构。

### 2.2 手动创建数据库

先用具有建库权限的 PostgreSQL 账号执行：

```sql
CREATE DATABASE "companion-service";
\c "companion-service"
CREATE EXTENSION IF NOT EXISTS vector;
```

然后选择数据库并执行表结构：

```sql
\i /Users/gaoyong/Documents/work/xinyuan_tech/companion-service/docs/sql/companion-service.sql
```

也可以使用 `psql` 直接执行：

```bash
psql "$COMPANION_DATABASE_DSN" \
  -f /Users/gaoyong/Documents/work/xinyuan_tech/companion-service/docs/sql/companion-service.sql
```

如果数据库尚未创建，需要先执行上面的 `CREATE DATABASE`；数据库角色必须拥有 `vector` 扩展和表的创建权限。

### 2.3 当前表

`docs/sql/companion-service.sql` 当前创建四张表：

| 表 | 用途 |
| --- | --- |
| `companion_conversation` | 保存内部上下文段归属、陪伴角色、状态、上下文摘要和破冰阶段 |
| `companion_message` | 保存用户消息和陪伴回复 |
| `companion_message_asset` | 保存图片/视频的资产引用和访问地址，二进制不落库 |
| `companion_memory` | 保存异步抽取的 L1 偏好、事实和目标记忆；Embedding 暂不可用时 `embedding` 可暂为空 |

字段、索引、外键和每个字段的业务含义以 [sql/companion-service.sql](sql/companion-service.sql) 为准。

### 2.4 删除和外键注意事项

- `companion_message.conversation_id` 外键保证消息只能属于已存在的会话。
- 当前外键没有配置 `ON DELETE CASCADE`。
- 当前服务不提供账号级数据导出或删除能力；仅支持按消息来源撤销记忆。
- 如未来启用账号级数据治理，必须由独立运营系统设计可审计的删除任务，不能直接复用用户侧接口。
- `companion_memory.source_message_id` 当前只保存来源消息 ID，没有建立数据库外键，便于记忆生命周期和删除流程独立处理。

## 3. model-gateway

`model-gateway` 当前不读写任何数据库，也没有 SQL 文件、GORM Model 或数据库配置。它只读取环境变量和 YAML 配置：

- `MODEL_GATEWAY_API_KEY`：访问 DeepSeek 等模型供应商的密钥。
- `MODEL_GATEWAY_INBOUND_API_KEY`：上游服务访问网关的内部密钥。
- `provider.base_url`、模型名称和超时配置。

模型请求和流式响应在请求生命周期内处理，不在网关落库。供应商侧的调用日志、计费和审计数据属于后续运营与观测设计，不应被误认为当前已有数据库能力。

## 4. PostgreSQL 说明

当前 Companion 已使用 PostgreSQL + pgvector 保存记忆向量，并通过 `embedding <=> query::vector` 做 cosine 距离排序。debug 和生产配置都使用 RocketMQ 投递记忆任务；原始图片和视频由 `asset-service` 写入 OSS，PostgreSQL 只保存关联引用。

记忆正文与记忆向量采用分阶段写入：候选记忆通过安全规则后先写入 `companion_memory`，Embedding 成功且维度为 1536 时再写入 `embedding`；Embedding 供应商失败不会丢弃正文，向量召回暂时退化为重要性排序。要启用语义召回，`model-gateway` 必须配置兼容 Embedding endpoint 和 `MODEL_GATEWAY_EMBEDDING_API_KEY`。
