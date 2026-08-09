# Companion Service Test Status

## Progress

- [x] Research and acceptance checklist recorded.
- [x] Plan recorded.
- [ ] Pure contracts expanded.
- [ ] PostgreSQL/pgvector SQL contracts expanded.
- [ ] Queue and orchestration boundaries expanded.
- [ ] Coverage and race validation completed.

## Quality review

Assertions will verify exact serialized payloads, SQL predicates, ordering, response mapping, and lifecycle side effects. A final review will re-read every generated assertion against production behavior and record any remaining seam or environment limitation here.

## Current blockers

- The local Go build cache is outside the writable workspace in this sandbox; validation must use task-local `GOCACHE`/`GOMODCACHE`.
- Live PostgreSQL/pgvector and RocketMQ are intentionally excluded from unit tests; integration-test prerequisites remain a separate validation gate.
