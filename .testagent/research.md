# Companion Service Test Research

## Scope

The current repository is authoritative. This broad-scope test pass targets the Go source that implements conversation orchestration, PostgreSQL/pgvector data access, memory extraction and processing, model-gateway client boundaries, safety policy, SSE transport, and configuration helpers. Generated protobuf files and server startup wiring are excluded unless they expose a testable contract.

## Existing conventions

- Tests use the standard `testing` package and table-driven cases where inputs share behavior.
- Package-local tests are used for unexported parsing, vector formatting, and lifecycle helpers.
- External model, database, and queue boundaries must use deterministic fakes or SQL mocks; no network, PostgreSQL, RocketMQ, or provider credentials are unit-test prerequisites.
- Error assertions must verify the returned error and the externally observable side effect, not only that an error occurred.

## Target inventory

- `internal/biz/context.go`: context budget, memory context, first-meeting identity prompt.
- `internal/data/conversation.go`: model constructors, limits, ownership predicates, pgvector query and user deletion lifecycle.
- `internal/memory/extractor.go`, `processor.go`, `rocketmq.go`: safe candidate parsing, skip/blocked policy, queue boundaries and dimension filtering.
- `internal/safety/safety.go`: crisis classification and response contract.
- `internal/client/model_gateway.go`: RPC request/stream lifecycle and cancellation.
- `internal/server/http.go`: SSE encoding and error paths.
- `model-gateway/internal/provider/openai_compatible.go`: chat, SSE, embedding, STT, TTS, status/context errors and format defaults.
- `model-gateway/internal/biz/model_gateway.go`: request validation and provider-to-proto mapping for all capabilities.
- `model-gateway/internal/server/auth.go`: HTTP/gRPC API-key extraction and constant-time authorization behavior.

## Acceptance checklist

1. PostgreSQL/pgvector召回边界、向量格式、查询排序和数据生命周期有可执行的 SQL-contract tests.
2. RocketMQ configuration/queue boundaries and in-process memory queue full/disabled/blocked behavior are tested without a broker.
3. Audio STT/TTS and Embedding request/response mapping, multipart fields, defaults and provider errors are tested.
4. Chat orchestration, streaming cancellation, first-meeting identity, memory context and safety policy are tested.
5. User ownership and deletion/export boundaries are asserted.
6. Tests compile and pass with race detection where practical; coverage is reported per package and limitations are documented.
