# Companion Service Test Status

## Progress

- [x] Research and acceptance checklist recorded.
- [x] Plan recorded.
- [x] Pure contracts expanded (`context`, `extractor`, `safety`, model constructors).
- [x] PostgreSQL/pgvector SQL and vector-literal contracts expanded.
- [x] Queue boundaries expanded for disabled, blocked, full, and incomplete RocketMQ configuration.
- [x] Media message, OSS asset upload contract, multimodal content parts, context rolling, and asset proto conversion tests expanded.
- [x] Coverage and race validation completed for all testable internal packages.

## Quality review

Assertions will verify exact serialized payloads, SQL predicates, ordering, response mapping, and lifecycle side effects. A final review will re-read every generated assertion against production behavior and record any remaining seam or environment limitation here.

## Current blockers

- The local Go build cache is outside the writable workspace in this sandbox; validation must use task-local `GOCACHE`/`GOMODCACHE`.
- Live PostgreSQL/pgvector and RocketMQ are intentionally excluded from unit tests; integration-test prerequisites remain a separate validation gate.

## Added test evidence

- `TestListRelevantMemoriesGeneratesVectorSearchContract`
- `TestProcessorEnqueueHonorsDisabledInvalidBlockedAndFullBoundaries`
- `TestNewRocketMQJobQueueValidatesConfigurationBeforeConnecting`
- `TestBuildChatMessagesAddsFirstMeetingGuidanceOnlyForNewConversation`
- `TestParseCandidatesNormalizesKindsAndFiltersSensitiveTerms`
- `TestSendMediaMessageUploadsPersistsAndBuildsMultimodalRequest`
- `TestSendVideoMessageBuildsVideoContentPart`
- `TestRollContextIfNeededSummarizesOversizedHistoryAndSwallowsModelFailure`
- `TestAssetClientUploadValidatesAndAddsAppMetadata`
- `TestStoreCreateMessageWithAssetsUsesSingleTransaction`
- `TestRollContextIfNeededLeavesShortHistoryUntouched`
- `TestSendMediaMessageValidatesMediaAndUploadBoundaries`
- `TestModelGatewayClientDelegatesAllUnaryCapabilitiesAndAddsAPIKey`
- `TestModelGatewayClientChatStreamClonesRequestAndClosesContext`
- `TestStoreListMessagesRestoresChronologicalOrder`
- `TestStoreMemoryFallbackAndUpdateEmbedding`
- `TestNewProcessorRunsLocalQueueAndStopsCleanly`
- `TestServerConstructorsAcceptDefaultAndExplicitConfig`
- `TestCompanionServiceWrapsUsecaseResponses`
- `TestForLocaleReturnsStableLocalizedCatalog`
- `TestLocaleFromContextReadsLanguageMetadataAndFallsBack`
- `TestBuildChatMessagesForLocaleUsesLocalizedCatalog`
- `TestOnboardingStageTransitions`
- `TestBuildChatMessagesUsesOnboardingStagePrompt`
- `TestSendMessageDoesNotAdvanceOnModelFailure`
- `TestSendMessageDoesNotAdvanceOnEmptyModelResponse`
- `TestSendMessageCrisisSkipsModelAndReturnsSafetyReply`
- `TestSendMessageStreamPersistsCompletedAssistantAndEmitsChunks`
- `TestSendMessageStreamDoesNotAdvanceOnModelFailure`
- `TestSendMediaMessageUploadsPersistsAndBuildsMultimodalRequest`
- `TestStoreAdvanceOnboardingStageUpdatesOwnedActiveConversation`

## Validation evidence

- `GOCACHE=/tmp/companion-go-cache GOMODCACHE=/Users/gaoyong/go/pkg/mod go test ./...`: passed.
- `GOCACHE=/tmp/companion-go-cache GOMODCACHE=/Users/gaoyong/go/pkg/mod go test -race ./...`: passed.
- `GOCACHE=/tmp/companion-go-cache GOMODCACHE=/Users/gaoyong/go/pkg/mod go vet ./...`: passed.
- Package coverage (latest local run): `internal/biz` 79.6%, `internal/client` 81.5%, `internal/data` 59.3%, `internal/lexicon` 87.5%, `internal/memory` 71.1%, `internal/safety` 100.0%, `internal/server` 39.7%, `internal/service` 76.0%.
- Coverage for generated/config-only packages cannot be collected in the current local Go toolchain because the `covdata` tool is unavailable; this is recorded as an environment limitation, not hidden.
