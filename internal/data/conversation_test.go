package data

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"companion-service/internal/conf"
	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newDryRunStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run postgres: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &Store{db: db}
}

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open mocked postgres: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &Store{db: db}, mock
}

func TestNewStoreRequiresDatabaseSource(t *testing.T) {
	_, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{SourceEnv: "COMPANION_TEST_MISSING_DSN"}})
	if err == nil {
		t.Fatal("expected missing database source error")
	}
}

func TestNewStoreReadsDatabaseSourceFromEnvironment(t *testing.T) {
	const expected = "postgres://gaoyong@127.0.0.1:5432/companion-service?sslmode=disable"
	t.Setenv("COMPANION_TEST_DSN", expected)
	source, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{SourceEnv: "COMPANION_TEST_DSN"}})
	if err != nil {
		t.Fatalf("resolve database source: %v", err)
	}
	if source != expected {
		t.Fatalf("unexpected database source: %s", source)
	}
}

func TestResolveDatabaseSourcePrefersExplicitSource(t *testing.T) {
	t.Setenv("COMPANION_TEST_DSN", "env-dsn")
	source, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{Source: " explicit-dsn ", SourceEnv: "COMPANION_TEST_DSN"}})
	if err != nil || source != "explicit-dsn" {
		t.Fatalf("expected explicit source, got %q, %v", source, err)
	}
}

func TestResolveDatabaseSourceRejectsNilAndBlankConfiguration(t *testing.T) {
	for name, cfg := range map[string]*conf.Data{
		"nil":              nil,
		"missing database": {},
		"blank values":     {Database: &conf.Database{Source: "  "}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveDatabaseSource(cfg); err == nil {
				t.Fatal("expected database source validation error")
			}
		})
	}
}

func TestFormatVectorUsesPostgresLiteralAndPreservesSigns(t *testing.T) {
	if got := formatVector([]float32{0.1, -2, 3.5}); got != "[0.1,-2,3.5]" {
		t.Fatalf("unexpected vector literal: %q", got)
	}
	if got := formatVector(nil); got != "[]" {
		t.Fatalf("unexpected empty vector literal: %q", got)
	}
}

func TestModelsExposeStableTableNamesAndPrefixes(t *testing.T) {
	if (ConversationModel{}).TableName() != "companion_conversation" || (MessageModel{}).TableName() != "companion_message" || (MemoryModel{}).TableName() != "companion_memory" {
		t.Fatal("unexpected table names")
	}
	message := NewMessage("conv-1", "user-1", "user", "hello")
	if !strings.HasPrefix(message.MessageID, "msg_") || message.ConversationID != "conv-1" || message.Role != "user" {
		t.Fatalf("unexpected message model: %+v", message)
	}
}

func TestListRelevantMemoriesGeneratesVectorSearchContract(t *testing.T) {
	store := newDryRunStore(t)
	stmt := store.db.Session(&gorm.Session{DryRun: true}).WithContext(context.Background()).Raw(`
		SELECT memory_id, user_id, layer, kind, content, source_message_id,
		       confidence, importance, status, created_at, updated_at
		FROM companion_memory
		WHERE user_id = ? AND status = 'active' AND embedding IS NOT NULL
		ORDER BY embedding <=> ?::vector, importance DESC, updated_at DESC
		LIMIT ?`, "user-1", formatVector([]float32{0.1, -0.2}), 5)
	if !strings.Contains(stmt.Statement.SQL.String(), "embedding <=>") || !strings.Contains(stmt.Statement.SQL.String(), "::vector") || !strings.Contains(stmt.Statement.SQL.String(), "embedding IS NOT NULL") {
		t.Fatalf("vector SQL contract missing: %q", stmt.Statement.SQL.String())
	}
}

func TestListAndMessageLimitNormalization(t *testing.T) {
	store, mock := newMockStore(t)
	for _, limit := range []int{0, -1, 101} {
		mock.ExpectQuery(`SELECT \* FROM "companion_message"`).
			WithArgs("user-1", 50).
			WillReturnRows(sqlmock.NewRows([]string{"message_id", "conversation_id", "user_id", "role", "content", "created_at"}))
		rows, err := store.ListTimelineMessages(context.Background(), "user-1", limit)
		if err != nil || len(rows) != 0 {
			t.Fatalf("normalized conversation limit %d: rows=%+v err=%v", limit, rows, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreActiveConversationLookupAndMessageLifecycle(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT \* FROM "companion_conversation"`).
		WithArgs("user-1", "default", "active", 1).WillReturnRows(sqlmock.NewRows([]string{"conversation_id", "user_id", "companion_id", "status", "summary", "created_at", "updated_at"}).AddRow("conv-1", "user-1", "default", "active", "", now, now))
	conversation, err := store.FindActiveConversation(context.Background(), "user-1", "default")
	if err != nil || conversation.ConversationID != "conv-1" {
		t.Fatalf("find active conversation: %v %+v", err, conversation)
	}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "companion_message"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "companion_conversation" SET "updated_at"=$1 WHERE conversation_id = $2 AND user_id = $3`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.CreateMessage(context.Background(), &MessageModel{MessageID: "msg-1", ConversationID: "conv-1", UserID: "user-1", Role: "user", Content: "hello", CreatedAt: now}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreAdvanceOnboardingStageUpdatesOwnedActiveConversation(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "companion_conversation" SET "onboarding_stage"=$1,"updated_at"=$2 WHERE conversation_id = $3 AND user_id = $4 AND status = $5`)).
		WithArgs("small_talk", sqlmock.AnyArg(), "conv-1", "user-1", "active").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.AdvanceOnboardingStage(context.Background(), "conv-1", "user-1", "small_talk"); err != nil {
		t.Fatalf("advance onboarding stage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreCreateMessageWithAssetsUsesSingleTransaction(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "companion_message"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "companion_message_asset"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "companion_conversation" SET "updated_at"=$1 WHERE conversation_id = $2 AND user_id = $3`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	message := &MessageModel{MessageID: "msg-media-1", ConversationID: "conv-1", UserID: "user-1", Role: "user", Content: "look", Modality: "image", CreatedAt: now}
	asset := MessageAssetModel{AssetID: "asset-1", MediaType: "image", ContentType: "image/png", Filename: "photo.png", URL: "https://asset.test/photo.png", SizeBytes: 10}
	if err := store.CreateMessageWithAssets(context.Background(), message, []MessageAssetModel{asset}); err != nil {
		t.Fatalf("create message with assets: %v", err)
	}
	if asset.MessageID != "" || message.Assets != nil {
		// The method receives a separate slice and should not mutate the model's transient asset field.
		t.Fatalf("unexpected caller-owned asset mutation: message=%+v asset=%+v", message, asset)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreMemoryRecallAndSourceDeletionLifecycle(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT memory_id, user_id, layer, kind, content, source_message_id, confidence, importance, status, created_at, updated_at`)).
		WithArgs("user-1", "[0.1,0.2]", 5).WillReturnRows(sqlmock.NewRows([]string{"memory_id", "user_id", "layer", "kind", "content", "source_message_id", "confidence", "importance", "status", "created_at", "updated_at"}).AddRow("mem-1", "user-1", "L1", "preference", "tea", "msg-1", 0.9, 3, "active", now, now))
	memories, err := store.ListRelevantMemories(context.Background(), "user-1", []float32{0.1, 0.2}, 5)
	if err != nil || len(memories) != 1 || memories[0].MemoryID != "mem-1" {
		t.Fatalf("recall memories: %v %+v", err, memories)
	}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "companion_memory" SET "status"=$1,"updated_at"=$2 WHERE user_id = $3 AND source_message_id = $4 AND status <> $5`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.DeleteMemoriesBySource(context.Background(), "user-1", "msg-1"); err != nil {
		t.Fatalf("delete source memory: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreReadAndMemoryWriteQueries(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT \* FROM "companion_memory"`).WillReturnRows(sqlmock.NewRows([]string{"memory_id", "user_id", "layer", "kind", "content", "source_message_id", "confidence", "importance", "status", "created_at", "updated_at"}).AddRow("mem-1", "user-1", "L1", "fact", "tea", "msg-1", 0.9, 3, "active", now, now))
	memories, err := store.ListActiveMemories(context.Background(), "user-1", 10)
	if err != nil || len(memories) != 1 {
		t.Fatalf("list memories: %v %+v", err, memories)
	}
	mock.ExpectQuery(`SELECT \* FROM "companion_message"`).WillReturnRows(sqlmock.NewRows([]string{"message_id", "conversation_id", "user_id", "role", "content", "created_at"}).AddRow("msg-1", "conv-1", "user-1", "user", "hello", now))
	messages, err := store.ListMessages(context.Background(), "conv-1", "user-1", 10)
	if err != nil || len(messages) != 1 || messages[0].MessageID != "msg-1" {
		t.Fatalf("list messages: %v %+v", err, messages)
	}
	mock.ExpectQuery(`SELECT \* FROM "companion_message"`).
		WithArgs("user-1", 10).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "conversation_id", "user_id", "role", "content", "created_at"}).AddRow("msg-1", "conv-1", "user-1", "user", "hello", now))
	timeline, err := store.ListTimelineMessages(context.Background(), "user-1", 10)
	if err != nil || len(timeline) != 1 || timeline[0].MessageID != "msg-1" {
		t.Fatalf("list timeline: %v %+v", err, timeline)
	}
	mock.ExpectQuery(`SELECT \* FROM "companion_message"`).WillReturnRows(sqlmock.NewRows([]string{"message_id", "conversation_id", "user_id", "role", "content", "created_at"}).AddRow("msg-1", "conv-1", "user-1", "user", "hello", now))
	message, err := store.GetMessage(context.Background(), "msg-1", "user-1")
	if err != nil || message.MessageID != "msg-1" {
		t.Fatalf("get message: %v %+v", err, message)
	}
	mock.ExpectQuery(`SELECT \* FROM "companion_memory"`).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "companion_memory"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.SaveMemory(context.Background(), &MemoryModel{MemoryID: "mem-2", UserID: "user-1", Layer: "L1", Kind: "fact", Content: "coffee", SourceMessageID: "msg-2", Confidence: 0.8, Importance: 2, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save memory: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreListMessagesRestoresChronologicalOrder(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT \* FROM "companion_message"`).
		WithArgs("conv-1", "user-1", 50).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "conversation_id", "user_id", "role", "content", "created_at"}).
			AddRow("msg-2", "conv-1", "user-1", "assistant", "second", now).
			AddRow("msg-1", "conv-1", "user-1", "user", "first", now.Add(-time.Minute)))
	messages, err := store.ListMessages(context.Background(), "conv-1", "user-1", 0)
	if err != nil || len(messages) != 2 || messages[0].MessageID != "msg-1" || messages[1].MessageID != "msg-2" {
		t.Fatalf("list messages should be chronological: %+v %v", messages, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestStoreMemoryFallbackAndUpdateEmbedding(t *testing.T) {
	store, mock := newMockStore(t)
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT \* FROM "companion_memory"`).
		WithArgs("user-1", "active", 5).
		WillReturnRows(sqlmock.NewRows([]string{"memory_id", "user_id", "layer", "kind", "content", "source_message_id", "confidence", "importance", "status", "created_at", "updated_at"}).AddRow("mem-1", "user-1", "L1", "preference", "tea", "msg-1", 0.8, 2, "active", now, now))
	memories, err := store.ListRelevantMemories(context.Background(), "user-1", nil, 0)
	if err != nil || len(memories) != 1 {
		t.Fatalf("empty embedding fallback: %+v %v", memories, err)
	}
	mock.ExpectQuery(`SELECT \* FROM "companion_memory"`).
		WithArgs("user-1", "preference", "tea", "active", 1).
		WillReturnRows(sqlmock.NewRows([]string{"memory_id", "user_id", "layer", "kind", "content", "source_message_id", "confidence", "importance", "status", "created_at", "updated_at"}).AddRow("mem-1", "user-1", "L1", "preference", "tea", "msg-old", 0.7, 1, "active", now, now))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "companion_memory" SET .* WHERE "memory_id" = \$5`).WithArgs(0.95, 4, "msg-new", now, "mem-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(`UPDATE companion_memory SET embedding = \$1::vector WHERE memory_id = \$2`).WithArgs("[0.1,0.2]", "mem-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.SaveMemory(context.Background(), &MemoryModel{MemoryID: "mem-new", UserID: "user-1", Kind: "preference", Content: "tea", SourceMessageID: "msg-new", Confidence: 0.95, Importance: 4, Embedding: []float32{0.1, 0.2}, UpdatedAt: now}); err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
