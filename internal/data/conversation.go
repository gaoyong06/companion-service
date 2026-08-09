package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"companion-service/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type ConversationModel struct {
	// 会话唯一标识，由服务端生成，通常使用 conv_ 前缀。
	ConversationID string `gorm:"primaryKey;size:64"`
	// 所属用户唯一标识，用于数据隔离、权限校验和用户级删除。
	UserID string `gorm:"index;size:64;not null"`
	// 陪伴角色唯一标识，当前默认值为 default。
	CompanionID string `gorm:"size:64;not null"`
	// 会话状态，当前使用 active 和 closed。
	Status string `gorm:"size:16;not null;index"`
	// 会话摘要，关闭会话时由模型生成。
	Summary string `gorm:"type:text"`
	// 会话创建时间，使用 UTC 时间。
	CreatedAt time.Time `gorm:"not null"`
	// 会话最后更新时间，新消息写入或会话关闭时更新。
	UpdatedAt time.Time `gorm:"not null"`
}

func (ConversationModel) TableName() string { return "companion_conversation" }

type MessageModel struct {
	// 消息唯一标识，由服务端生成，通常使用 msg_ 前缀。
	MessageID string `gorm:"primaryKey;size:64"`
	// 所属会话唯一标识。
	ConversationID string `gorm:"index;size:64;not null"`
	// 所属用户唯一标识，冗余保存以支持用户范围查询和删除。
	UserID string `gorm:"index;size:64;not null"`
	// 消息角色，当前使用 user、assistant 和 system。
	Role string `gorm:"size:16;not null"`
	// 消息正文，保存用户输入或模型生成的 UTF-8 文本。
	Content string `gorm:"type:text;not null"`
	// 消息创建时间，使用 UTC 时间。
	CreatedAt time.Time `gorm:"not null;index"`
}

func (MessageModel) TableName() string { return "companion_message" }

type MemoryModel struct {
	// 记忆唯一标识，由服务端生成，通常使用 mem_ 前缀。
	MemoryID string `gorm:"primaryKey;size:64"`
	// 所属用户唯一标识，记忆只能在该用户的上下文中召回。
	UserID string `gorm:"index;size:64;not null"`
	// 记忆层级，当前使用 L1。
	Layer string `gorm:"size:16;not null"`
	// 记忆类型，当前支持 preference、fact 和 goal。
	Kind string `gorm:"size:32;not null"`
	// 记忆正文，不得保存密码、令牌、银行卡等敏感信息。
	Content string `gorm:"type:text;not null"`
	// 产生该记忆的源消息唯一标识，用于反馈纠正和生命周期追踪。
	SourceMessageID string `gorm:"index;size:64;not null"`
	// 模型对记忆内容正确性的置信度，范围为 0.0 至 1.0。
	Confidence float64 `gorm:"not null"`
	// 记忆重要性评分，当前为 1 至 5 的整数。
	Importance int32 `gorm:"not null"`
	// 记忆状态，当前使用 active 和 deleted。
	Status string `gorm:"size:16;not null;index"`
	// 记忆首次创建时间，使用 UTC 时间。
	CreatedAt time.Time `gorm:"not null"`
	// 记忆最后更新时间，使用 UTC 时间。
	UpdatedAt time.Time `gorm:"not null"`
	// Embedding 是用于向量召回的临时向量，不直接由 GORM 自动持久化。
	Embedding []float32 `gorm:"-"`
}

func (MemoryModel) TableName() string { return "companion_memory" }

type Store struct {
	db *gorm.DB
}

func NewStore(c *conf.Data, logger log.Logger) (*Store, func(), error) {
	source, err := resolveDatabaseSource(c)
	if err != nil {
		return nil, nil, err
	}
	db, err := gorm.Open(postgres.Open(source), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get database handle: %w", err)
	}
	sqlDB.SetMaxIdleConns(int(c.Database.MaxIdleConns))
	sqlDB.SetMaxOpenConns(int(c.Database.MaxOpenConns))
	log.NewHelper(logger).Info("database connection established")
	return &Store{db: db}, func() { _ = sqlDB.Close() }, nil
}

func resolveDatabaseSource(c *conf.Data) (string, error) {
	if c == nil || c.Database == nil {
		return "", errors.New("database source is required")
	}
	source := strings.TrimSpace(c.Database.Source)
	if source == "" && c.Database.SourceEnv != "" {
		source = strings.TrimSpace(os.Getenv(c.Database.SourceEnv))
	}
	if source == "" {
		return "", errors.New("database source is required")
	}
	return source, nil
}

func (s *Store) CreateConversation(ctx context.Context, model *ConversationModel) error {
	return s.db.WithContext(ctx).Create(model).Error
}

func (s *Store) GetConversation(ctx context.Context, conversationID, userID string) (*ConversationModel, error) {
	var model ConversationModel
	err := s.db.WithContext(ctx).Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&model).Error
	return &model, err
}

func (s *Store) CloseConversation(ctx context.Context, conversationID, userID, summary string, updatedAt time.Time) (*ConversationModel, error) {
	result := s.db.WithContext(ctx).Model(&ConversationModel{}).
		Where("conversation_id = ? AND user_id = ? AND status = ?", conversationID, userID, "active").
		Updates(map[string]interface{}{"status": "closed", "summary": summary, "updated_at": updatedAt})
	if result.Error != nil {
		return nil, result.Error
	}
	return s.GetConversation(ctx, conversationID, userID)
}

func (s *Store) ListConversations(ctx context.Context, userID string, limit int) ([]ConversationModel, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []ConversationModel
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (s *Store) ListActiveMemories(ctx context.Context, userID string, limit int) ([]MemoryModel, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var rows []MemoryModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, "active").
		Order("importance DESC, updated_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// ListRelevantMemories 按余弦距离召回用户的高相关记忆；没有向量时退化为重要性排序。
func (s *Store) ListRelevantMemories(ctx context.Context, userID string, embedding []float32, limit int) ([]MemoryModel, error) {
	if len(embedding) == 0 {
		return s.ListActiveMemories(ctx, userID, limit)
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	vector := formatVector(embedding)
	var rows []MemoryModel
	err := s.db.WithContext(ctx).Raw(`
		SELECT memory_id, user_id, layer, kind, content, source_message_id,
		       confidence, importance, status, created_at, updated_at
		FROM companion_memory
		WHERE user_id = ? AND status = 'active' AND embedding IS NOT NULL
		ORDER BY embedding <=> ?::vector, importance DESC, updated_at DESC
		LIMIT ?`, userID, vector, limit).Scan(&rows).Error
	return rows, err
}

func (s *Store) ExportUserData(ctx context.Context, userID string) ([]ConversationModel, []MessageModel, []MemoryModel, error) {
	var conversations []ConversationModel
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&conversations).Error; err != nil {
		return nil, nil, nil, err
	}
	var messages []MessageModel
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, nil, nil, err
	}
	var memories []MemoryModel
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&memories).Error; err != nil {
		return nil, nil, nil, err
	}
	return conversations, messages, memories, nil
}

func (s *Store) SaveMemory(ctx context.Context, model *MemoryModel) error {
	var existing MemoryModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND kind = ? AND content = ? AND status = ?", model.UserID, model.Kind, model.Content, "active").
		First(&existing).Error
	if err == nil {
		if updateErr := s.db.WithContext(ctx).Model(&existing).Updates(map[string]interface{}{
			"source_message_id": model.SourceMessageID,
			"confidence":        model.Confidence,
			"importance":        model.Importance,
			"updated_at":        model.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		return s.saveMemoryEmbedding(ctx, existing.MemoryID, model.Embedding)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := s.db.WithContext(ctx).Omit("Embedding").Create(model).Error; err != nil {
		return err
	}
	return s.saveMemoryEmbedding(ctx, model.MemoryID, model.Embedding)
}

func (s *Store) saveMemoryEmbedding(ctx context.Context, memoryID string, embedding []float32) error {
	if len(embedding) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Exec("UPDATE companion_memory SET embedding = ?::vector WHERE memory_id = ?", formatVector(embedding), memoryID).Error
}

func formatVector(embedding []float32) string {
	values := make([]string, len(embedding))
	for index, value := range embedding {
		values[index] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return "[" + strings.Join(values, ",") + "]"
}

func (s *Store) DeleteMemoriesBySource(ctx context.Context, userID, sourceMessageID string) error {
	return s.db.WithContext(ctx).Model(&MemoryModel{}).
		Where("user_id = ? AND source_message_id = ? AND status <> ?", userID, sourceMessageID, "deleted").
		Updates(map[string]interface{}{"status": "deleted", "updated_at": time.Now().UTC()}).Error
}

func (s *Store) DeleteUserData(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&MemoryModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&MessageModel{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&ConversationModel{}).Error
	})
}

func (s *Store) ListMessages(ctx context.Context, conversationID, userID string, limit int) ([]MessageModel, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []MessageModel
	err := s.db.WithContext(ctx).Where("conversation_id = ? AND user_id = ?", conversationID, userID).Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, nil
}

func (s *Store) GetMessage(ctx context.Context, messageID, conversationID, userID string) (*MessageModel, error) {
	var model MessageModel
	err := s.db.WithContext(ctx).
		Where("message_id = ? AND conversation_id = ? AND user_id = ?", messageID, conversationID, userID).
		First(&model).Error
	return &model, err
}

func (s *Store) CreateMessage(ctx context.Context, model *MessageModel) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		return tx.Model(&ConversationModel{}).
			Where("conversation_id = ? AND user_id = ?", model.ConversationID, model.UserID).
			Update("updated_at", model.CreatedAt).Error
	})
}

func (s *Store) NewConversation(userID, companionID string) *ConversationModel {
	now := time.Now().UTC()
	return &ConversationModel{ConversationID: "conv_" + uuid.NewString(), UserID: userID, CompanionID: companionID, Status: "active", CreatedAt: now, UpdatedAt: now}
}

func NewMessage(conversationID, userID, role, content string) *MessageModel {
	return &MessageModel{MessageID: "msg_" + uuid.NewString(), ConversationID: conversationID, UserID: userID, Role: role, Content: content, CreatedAt: time.Now().UTC()}
}
