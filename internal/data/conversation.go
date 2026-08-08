package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"companion-service/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

type ConversationModel struct {
	ConversationID string    `gorm:"primaryKey;size:64"`
	UserID         string    `gorm:"index;size:64;not null"`
	CompanionID    string    `gorm:"size:64;not null"`
	Status         string    `gorm:"size:16;not null;index"`
	Summary        string    `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (ConversationModel) TableName() string { return "companion_conversation" }

type MessageModel struct {
	MessageID      string    `gorm:"primaryKey;size:64"`
	ConversationID string    `gorm:"index;size:64;not null"`
	UserID         string    `gorm:"index;size:64;not null"`
	Role           string    `gorm:"size:16;not null"`
	Content        string    `gorm:"type:text;not null"`
	CreatedAt      time.Time `gorm:"not null;index"`
}

func (MessageModel) TableName() string { return "companion_message" }

type MemoryModel struct {
	MemoryID        string    `gorm:"primaryKey;size:64"`
	UserID          string    `gorm:"index;size:64;not null"`
	Layer           string    `gorm:"size:16;not null"`
	Kind            string    `gorm:"size:32;not null"`
	Content         string    `gorm:"type:text;not null"`
	SourceMessageID string    `gorm:"index;size:64;not null"`
	Confidence      float64   `gorm:"not null"`
	Importance      int32     `gorm:"not null"`
	Status          string    `gorm:"size:16;not null;index"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
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
	db, err := gorm.Open(mysql.Open(source), &gorm.Config{})
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

func (s *Store) GetConversation(ctx context.Context, id, userID string) (*ConversationModel, error) {
	var model ConversationModel
	err := s.db.WithContext(ctx).Where("conversation_id = ? AND user_id = ?", id, userID).First(&model).Error
	return &model, err
}

func (s *Store) CloseConversation(ctx context.Context, id, userID, summary string, updatedAt time.Time) (*ConversationModel, error) {
	result := s.db.WithContext(ctx).Model(&ConversationModel{}).
		Where("conversation_id = ? AND user_id = ? AND status = ?", id, userID, "active").
		Updates(map[string]interface{}{"status": "closed", "summary": summary, "updated_at": updatedAt})
	if result.Error != nil {
		return nil, result.Error
	}
	return s.GetConversation(ctx, id, userID)
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
		return s.db.WithContext(ctx).Model(&existing).Updates(map[string]interface{}{
			"source_message_id": model.SourceMessageID,
			"confidence":        model.Confidence,
			"importance":        model.Importance,
			"updated_at":        model.UpdatedAt,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(model).Error
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
