# 数据库说明

> 文档状态：Current MVP
> 更新日期：2026-08-08

## 1. 数据库清单

| 服务 | 数据库 | 当前用途 | 是否需要手动建库 |
| --- | --- | --- | --- |
| `companion-service` | MySQL 8.x（兼容 MySQL 5.7 的基础语法） | 会话、消息、L1 记忆持久化 | 是 |
| `model-gateway` | 无 | 无状态转发 Chat、Embedding 和模型列表请求 | 否 |

当前版本没有启用 PostgreSQL、pgvector、Redis、对象存储或消息队列。它们只出现在技术规划中，尚未接入代码、配置或 SQL 表结构。

## 2. companion-service

### 2.1 数据库名称和连接

Debug 配置默认连接：

```text
root:root@tcp(127.0.0.1:3306)/companion-service?charset=utf8mb4&parseTime=True&loc=Local
```

Release 配置从环境变量 `COMPANION_DATABASE_DSN` 读取完整 DSN，例如：

```bash
export COMPANION_DATABASE_DSN='app_user:strong-password@tcp(mysql:3306)/companion-service?charset=utf8mb4&parseTime=True&loc=UTC'
```

连接使用 GORM MySQL 驱动，代码入口为 `internal/data/conversation.go`。服务启动时不会自动创建数据库，也不会自动迁移表结构。

### 2.2 手动创建数据库

先用具有建库权限的 MySQL 账号执行：

```sql
CREATE DATABASE IF NOT EXISTS `companion-service`
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;
```

然后选择数据库并执行表结构：

```sql
USE `companion-service`;
SOURCE /Users/gaoyong/Documents/work/xinyuan_tech/companion-service/docs/sql/companion-service.sql;
```

也可以在 MySQL 客户端中直接打开并执行：

```bash
mysql -h 127.0.0.1 -u root -p 'companion-service' \
  < /Users/gaoyong/Documents/work/xinyuan_tech/companion-service/docs/sql/companion-service.sql
```

如果数据库尚未创建，需要先执行上面的 `CREATE DATABASE`。

### 2.3 当前表

`docs/sql/companion-service.sql` 当前创建三张表：

| 表 | 用途 |
| --- | --- |
| `companion_conversation` | 保存会话归属、陪伴角色、状态和会话摘要 |
| `companion_message` | 保存用户消息和陪伴回复 |
| `companion_memory` | 保存异步抽取的 L1 偏好、事实和目标记忆 |

字段、索引、外键和每个字段的业务含义以 [sql/companion-service.sql](sql/companion-service.sql) 为准。

### 2.4 删除和外键注意事项

- `companion_message.conversation_id` 外键保证消息只能属于已存在的会话。
- 当前外键没有配置 `ON DELETE CASCADE`。
- 账号删除由服务事务显式删除消息、会话和记忆，不能只删除会话主表。
- `companion_memory.source_message_id` 当前只保存来源消息 ID，没有建立数据库外键，便于记忆生命周期和删除流程独立处理。

## 3. model-gateway

`model-gateway` 当前不读写任何数据库，也没有 SQL 文件、GORM Model 或数据库配置。它只读取环境变量和 YAML 配置：

- `MODEL_GATEWAY_API_KEY`：访问 DeepSeek 等模型供应商的密钥。
- `MODEL_GATEWAY_INBOUND_API_KEY`：上游服务访问网关的内部密钥。
- `provider.base_url`、模型名称和超时配置。

模型请求和流式响应在请求生命周期内处理，不在网关落库。供应商侧的调用日志、计费和审计数据属于后续运营与观测设计，不应被误认为当前已有数据库能力。

## 4. PostgreSQL 说明

`companion-service/docs/tech.md` 原先将 PostgreSQL、JSONB 和 pgvector 写在 MVP 推荐表中，这描述的是生产演进方向，不是当前实现。当前版本：

- 没有 PostgreSQL 驱动或连接配置。
- 没有 pgvector 扩展配置。
- 没有 PostgreSQL DDL 或对应 Repository 实现。
- 没有 Redis、对象存储或消息队列的运行依赖。

因此本阶段不需要创建 PostgreSQL 数据库。等 L2/L3 记忆、向量召回和异步任务真正进入开发时，再单独评估是否迁移到 PostgreSQL + pgvector，并同步补充迁移脚本和表设计。
