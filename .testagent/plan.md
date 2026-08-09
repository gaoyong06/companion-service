# Companion Service Test Plan

## Phase 1: deterministic pure contracts

- Extend context tests for empty/invalid history, stale/empty memories, and budget behavior.
- Extend extractor/safety tests for malformed JSON, sensitive terms, normalization, and all crisis response branches.
- Add data helper tests for vector serialization, model table names, constructor prefixes, and database source precedence.

## Phase 2: persistence and lifecycle contracts

- Add `sqlmock`-style SQL contract coverage if dependency is available; otherwise use GORM DryRun to assert PostgreSQL pgvector SQL and ownership predicates.
- Cover active-memory fallback, vector query, limit normalization, close/update, message order, export ordering, and transactional deletion semantics.

## Phase 3: queue and orchestration boundaries

- Cover processor disabled/skip/blocked/full paths and embedding-dimension mismatch with fakes where concrete dependency seams permit.
- Cover RocketMQ configuration validation without publishing to a live broker.
- Cover client cancellation and stream receive behavior.

## Phase 4: model gateway

- Add provider tests for malformed/empty responses, context cancellation, default filenames/content types, STT/TTS fields, embedding errors and HTTP status details.
- Add usecase tests for every capability's validation, provider delegation, response mapping and model listing.
- Add API-key tests for HTTP transport headers, gRPC metadata and missing configuration.

## Explicit blockers

- `companion-service` usecase and processor currently accept concrete `*data.Store` and `*ModelGatewayClient`; broad end-to-end orchestration tests cannot inject fakes without a production interface refactor. This pass covers their testable pure and boundary layers and records the seam as residual risk instead of introducing test-only production behavior.
