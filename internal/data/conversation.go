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

func (s *Store) CreateMessage(ctx context.Context, model *MessageModel) error {
	return s.db.WithContext(ctx).Create(model).Error
}

func (s *Store) NewConversation(userID, companionID string) *ConversationModel {
	now := time.Now().UTC()
	return &ConversationModel{ConversationID: "conv_" + uuid.NewString(), UserID: userID, CompanionID: companionID, Status: "active", CreatedAt: now, UpdatedAt: now}
}

func NewMessage(conversationID, userID, role, content string) *MessageModel {
	return &MessageModel{MessageID: "msg_" + uuid.NewString(), ConversationID: conversationID, UserID: userID, Role: role, Content: content, CreatedAt: time.Now().UTC()}
}
