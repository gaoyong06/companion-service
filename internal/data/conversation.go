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
	"gorm.io/gorm/clause"
)

var ErrNotFound = gorm.ErrRecordNotFound

type ConversationModel struct {
	// 会话唯一标识，由服务端生成，通常使用 conv_ 前缀。
	ConversationID string `gorm:"primaryKey;size:64"`
	// 所属用户唯一标识，用于数据隔离和权限校验。
	UserID string `gorm:"index;size:64;not null"`
	// 陪伴角色唯一标识，当前默认值为 default。
	CompanionID string `gorm:"size:64;not null"`
	// 内部上下文段状态，当前使用 active 和 closed。
	Status string `gorm:"size:16;not null;index"`
	// 内部上下文段摘要，由系统在上下文预算达到阈值时生成。
	Summary string `gorm:"type:text"`
	// 破冰阶段，由服务端推进，避免模型自行跳过社交节奏。
	OnboardingStage string `gorm:"size:32;not null;default:first_meeting"`
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
	// 所属用户唯一标识，冗余保存以支持用户范围查询和数据治理。
	UserID string `gorm:"index;size:64;not null"`
	// 消息角色，当前使用 user、assistant 和 system。
	Role string `gorm:"size:16;not null"`
	// 消息正文，保存用户输入或模型生成的 UTF-8 文本。
	Content string `gorm:"type:text;not null"`
	// 消息创建时间，使用 UTC 时间。
	CreatedAt time.Time `gorm:"not null;index"`
	// 消息载体类型，当前使用 text、audio、image 和 video。
	Modality string `gorm:"size:16;not null;default:text"`
	// 关联的媒体资产，仅由服务端加载和写入。
	Assets []MessageAssetModel `gorm:"-"`
}

func (MessageModel) TableName() string { return "companion_message" }

type MessageAssetModel struct {
	// 消息资产关联唯一标识。
	MessageAssetID string `gorm:"primaryKey;size:64"`
	// 所属消息唯一标识。
	MessageID string `gorm:"index;size:64;not null"`
	// asset-service 生成的文件唯一标识。
	AssetID string `gorm:"size:64;not null"`
	// 媒体类型，当前使用 image 或 video。
	MediaType string `gorm:"size:16;not null"`
	// 文件 MIME 类型。
	ContentType string `gorm:"size:128;not null"`
	// 用户上传时的原始文件名。
	Filename string `gorm:"size:255;not null"`
	// 资产访问地址。
	URL string `gorm:"type:text;not null"`
	// 文件大小，单位为字节。
	SizeBytes int64 `gorm:"not null"`
}

func (MessageAssetModel) TableName() string { return "companion_message_asset" }

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

func (s *Store) FindActiveConversation(ctx context.Context, userID, companionID string) (*ConversationModel, error) {
	var model ConversationModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND companion_id = ? AND status = ?", userID, companionID, "active").
		Order("updated_at DESC").
		First(&model).Error
	return &model, err
}

// GetOrCreateActiveConversation 为用户获取唯一的当前活动上下文，不要求客户端管理会话 ID。
func (s *Store) GetOrCreateActiveConversation(ctx context.Context, userID, companionID string) (*ConversationModel, error) {
	conversation, err := s.FindActiveConversation(ctx, userID, companionID)
	if err == nil {
		return conversation, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := time.Now().UTC()
	candidate := &ConversationModel{
		ConversationID: "conv_" + uuid.NewString(),
		UserID:         userID,
		CompanionID:    companionID,
		Status:         "active",
		OnboardingStage: "first_meeting",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(candidate).Error; err != nil {
		return nil, err
	}
	return s.FindActiveConversation(ctx, userID, companionID)
}

// RollActiveConversation 关闭旧上下文并创建带摘要的新上下文，保证用户时间线仍然连续。
func (s *Store) RollActiveConversation(ctx context.Context, conversation *ConversationModel, summary string) (*ConversationModel, error) {
	if conversation == nil || conversation.UserID == "" || conversation.CompanionID == "" {
		return nil, errors.New("conversation is required")
	}
	now := time.Now().UTC()
	var next ConversationModel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ConversationModel{}).
			Where("conversation_id = ? AND user_id = ? AND status = ?", conversation.ConversationID, conversation.UserID, "active").
			Updates(map[string]interface{}{"status": "closed", "summary": summary, "updated_at": now}).Error; err != nil {
			return err
		}
		next = ConversationModel{ConversationID: "conv_" + uuid.NewString(), UserID: conversation.UserID, CompanionID: conversation.CompanionID, Status: "active", Summary: summary, OnboardingStage: conversation.OnboardingStage, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&next).Error
	})
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *Store) AdvanceOnboardingStage(ctx context.Context, conversationID, userID, stage string) error {
	return s.db.WithContext(ctx).Model(&ConversationModel{}).
		Where("conversation_id = ? AND user_id = ? AND status = ?", conversationID, userID, "active").
		Updates(map[string]interface{}{"onboarding_stage": stage, "updated_at": time.Now().UTC()}).Error
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
	if err := s.loadMessageAssets(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListTimelineMessages 按用户聚合所有内部上下文段的消息，保持对用户可见的时间线连续。
func (s *Store) ListTimelineMessages(ctx context.Context, userID string, limit int) ([]MessageModel, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []MessageModel
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	if err := s.loadMessageAssets(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) loadMessageAssets(ctx context.Context, rows []MessageModel) error {
	if len(rows) == 0 {
		return nil
	}
	messageIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Modality == "" || row.Modality == "text" {
			continue
		}
		messageIDs = append(messageIDs, row.MessageID)
	}
	if len(messageIDs) == 0 {
		return nil
	}
	var assets []MessageAssetModel
	if err := s.db.WithContext(ctx).Where("message_id IN ?", messageIDs).Find(&assets).Error; err != nil {
		return err
	}
	byMessage := make(map[string][]MessageAssetModel, len(assets))
	for _, asset := range assets {
		byMessage[asset.MessageID] = append(byMessage[asset.MessageID], asset)
	}
	for index := range rows {
		rows[index].Assets = byMessage[rows[index].MessageID]
	}
	return nil
}

func (s *Store) GetMessage(ctx context.Context, messageID, userID string) (*MessageModel, error) {
	var model MessageModel
	err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ?", messageID, userID).
		First(&model).Error
	return &model, err
}

func (s *Store) CreateMessage(ctx context.Context, model *MessageModel) error {
	return s.CreateMessageWithAssets(ctx, model, model.Assets)
}

func (s *Store) CreateMessageWithAssets(ctx context.Context, model *MessageModel, assets []MessageAssetModel) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		if len(assets) > 0 {
			for index := range assets {
				assets[index].MessageID = model.MessageID
				if assets[index].MessageAssetID == "" {
					assets[index].MessageAssetID = "msg_asset_" + uuid.NewString()
				}
			}
			if err := tx.Create(&assets).Error; err != nil {
				return err
			}
		}
		return tx.Model(&ConversationModel{}).
			Where("conversation_id = ? AND user_id = ?", model.ConversationID, model.UserID).
			Update("updated_at", model.CreatedAt).Error
	})
}

func NewMessage(conversationID, userID, role, content string) *MessageModel {
	return &MessageModel{MessageID: "msg_" + uuid.NewString(), ConversationID: conversationID, UserID: userID, Role: role, Content: content, Modality: "text", CreatedAt: time.Now().UTC()}
}
