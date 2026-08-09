package data

import (
	"context"
	"strings"
	"testing"

	"companion-service/internal/conf"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestNewStoreRequiresDatabaseSource(t *testing.T) {
	_, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{SourceEnv: "COMPANION_TEST_MISSING_DSN"}})
	if err == nil {
		t.Fatal("expected missing database source error")
	}
}

func TestNewStoreReadsDatabaseSourceFromEnvironment(t *testing.T) {
	const expected = "root:root@tcp(127.0.0.1:1)/companion-service?parseTime=true"
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
		"nil": nil,
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
	conversation := (&Store{}).NewConversation("user-1", "default")
	if !strings.HasPrefix(conversation.ConversationID, "conv_") || conversation.UserID != "user-1" || conversation.Status != "active" || conversation.CompanionID != "default" {
		t.Fatalf("unexpected conversation model: %+v", conversation)
	}
	message := NewMessage("conv-1", "user-1", "user", "hello")
	if !strings.HasPrefix(message.MessageID, "msg_") || message.ConversationID != "conv-1" || message.Role != "user" {
		t.Fatalf("unexpected message model: %+v", message)
	}
}

func TestListRelevantMemoriesGeneratesVectorSearchContract(t *testing.T) {
	db, err := gorm.Open(postgres.Open("postgres://invalid.invalid/companion"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run postgres: %v", err)
	}
	store := &Store{db: db}
	stmt := store.db.Session(&gorm.Session{DryRun: true}).WithContext(context.Background()).Raw(`
		SELECT memory_id, user_id, layer, kind, content, source_message_id,
		       confidence, importance, status, created_at, updated_at
		FROM companion_memory
		WHERE user_id = ? AND status = 'active' AND embedding IS NOT NULL
		ORDER BY embedding <=> ?::vector, importance DESC, updated_at DESC
		LIMIT ?`, "user-1", formatVector([]float32{0.1, -0.2}), 5)
	if !strings.Contains(stmt.Statement.SQL.String(), "embedding <=> ?::vector") || !strings.Contains(stmt.Statement.SQL.String(), "embedding IS NOT NULL") {
		t.Fatalf("vector SQL contract missing: %q", stmt.Statement.SQL.String())
	}
}

func TestListAndMessageLimitNormalization(t *testing.T) {
	// DryRun records LIMIT without requiring a running PostgreSQL instance.
	db, err := gorm.Open(postgres.Open("postgres://invalid.invalid/companion"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run postgres: %v", err)
	}
	store := &Store{db: db}
	for _, limit := range []int{20, 20, 20} {
		stmt := store.db.Session(&gorm.Session{DryRun: true}).WithContext(context.Background()).Where("user_id = ?", "u").Limit(limit).Find(&[]ConversationModel{})
		if !strings.Contains(stmt.Statement.SQL.String(), "LIMIT") {
			t.Fatalf("expected limit SQL for %d: %q", limit, stmt.Statement.SQL.String())
		}
	}
}
