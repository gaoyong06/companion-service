package biz

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	assetv1 "asset-service/api/asset/v1"
	v1 "companion-service/api/companion/v1"
	"companion-service/internal/client"
	"companion-service/internal/data"
	"companion-service/internal/lexicon"
	"companion-service/internal/memory"
	"companion-service/internal/safety"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type CompanionUsecase struct {
	store  conversationStore
	models client.ModelGateway
	assets client.AssetStorage
	memory memoryProcessor
	log    *log.Helper
}

type conversationStore interface {
	GetOrCreateActiveConversation(context.Context, string, string) (*data.ConversationModel, error)
	ListMessages(context.Context, string, string, int) ([]data.MessageModel, error)
	ListTimelineMessages(context.Context, string, int) ([]data.MessageModel, error)
	GetMessage(context.Context, string, string) (*data.MessageModel, error)
	CreateMessage(context.Context, *data.MessageModel) error
	ListRelevantMemories(context.Context, string, []float32, int) ([]data.MemoryModel, error)
	DeleteMemoriesBySource(context.Context, string, string) error
	SaveMemory(context.Context, *data.MemoryModel) error
	AdvanceOnboardingStage(context.Context, string, string, string) error
}

type ConversationStore = conversationStore

type memoryProcessor interface {
	Enqueue(memory.Job) error
	ForgetUser(string)
}

type MemoryProcessor = memoryProcessor

const maxMessageContentLength = 16 * 1024

const companionMaxTokens int32 = 256

const contextRollCharacterThreshold = 18 * 1024

func NewCompanionUsecase(store conversationStore, models client.ModelGateway, assets client.AssetStorage, processor memoryProcessor, logger log.Logger) *CompanionUsecase {
	if logger == nil {
		logger = log.DefaultLogger
	}
	return &CompanionUsecase{store: store, models: models, assets: assets, memory: processor, log: log.NewHelper(logger)}
}

// enqueueMemory 将记忆任务放入异步流水线；队列异常不能影响已经完成的当前回复，但必须记录日志。
func (u *CompanionUsecase) enqueueMemory(job memory.Job) {
	if u == nil || u.memory == nil {
		return
	}
	if err := u.memory.Enqueue(job); err != nil {
		u.log.Warnf("enqueue memory job failed: %v", err)
	}
}

func (u *CompanionUsecase) activeConversation(ctx context.Context) (*data.ConversationModel, string, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, "", fmt.Errorf("user identity is required")
	}
	const companionID = "default"
	conversation, err := u.store.GetOrCreateActiveConversation(ctx, userID, companionID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve active companion context: %w", err)
	}
	return conversation, userID, nil
}

func (u *CompanionUsecase) rollContextIfNeeded(ctx context.Context, conversation *data.ConversationModel, history []data.MessageModel) (*data.ConversationModel, []data.MessageModel, error) {
	roller, ok := u.store.(interface {
		RollActiveConversation(context.Context, *data.ConversationModel, string) (*data.ConversationModel, error)
	})
	if !ok || conversation == nil || messageCharacters(history) < contextRollCharacterThreshold {
		return conversation, history, nil
	}
	locale := lexicon.LocaleFromContext(ctx)
	summaryMessages := make([]*modelv1.ChatMessage, 0, len(history)+1)
	summaryMessages = append(summaryMessages, &modelv1.ChatMessage{Role: "system", Content: lexicon.ForLocale(locale).Prompts.ConversationSummary})
	for _, message := range history {
		if message.Role != "" && strings.TrimSpace(message.Content) != "" {
			summaryMessages = append(summaryMessages, &modelv1.ChatMessage{Role: message.Role, Content: message.Content})
		}
	}
	result, err := u.models.Chat(ctx, &modelv1.ChatCompletionRequest{Messages: summaryMessages, MaxTokens: 384})
	if err != nil || strings.TrimSpace(result.Content) == "" {
		return conversation, history, nil
	}
	next, err := roller.RollActiveConversation(ctx, conversation, strings.TrimSpace(result.Content))
	if err != nil {
		return nil, nil, fmt.Errorf("roll companion context: %w", err)
	}
	return next, nil, nil
}

func messageCharacters(messages []data.MessageModel) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
}

func (u *CompanionUsecase) GetTimeline(ctx context.Context, req *v1.GetTimelineRequest) ([]data.MessageModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user identity is required")
	}
	limit := 50
	if req != nil && req.Limit > 0 {
		limit = int(req.Limit)
	}
	messages, err := u.store.ListTimelineMessages(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list companion timeline: %w", err)
	}
	return messages, nil
}

func (u *CompanionUsecase) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*data.MessageModel, *data.MessageModel, error) {
	content := ""
	if req != nil {
		content = strings.TrimSpace(req.Content)
	}
	if content == "" {
		return nil, nil, fmt.Errorf("user identity and content are required")
	}
	conversation, userID, err := u.activeConversation(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(content) > maxMessageContentLength {
		return nil, nil, fmt.Errorf("message content exceeds %d bytes", maxMessageContentLength)
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 100)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversation history: %w", err)
	}
	conversation, history, err = u.rollContextIfNeeded(ctx, conversation, history)
	if err != nil {
		return nil, nil, err
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", content)
	if err := u.store.CreateMessage(ctx, userMessage); err != nil {
		return nil, nil, fmt.Errorf("save user message: %w", err)
	}
	if level := safety.CheckForLocale(content, lexicon.LocaleFromContext(ctx)); level == safety.LevelCrisis {
		assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", safety.ResponseForLocale(level, lexicon.LocaleFromContext(ctx)))
		if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
			return userMessage, nil, fmt.Errorf("save safety response: %w", err)
		}
		return userMessage, assistantMessage, nil
	}
	embeddingReply, embeddingErr := u.models.Embed(ctx, []string{content})
	var queryEmbedding []float32
	if embeddingErr == nil && embeddingReply != nil && len(embeddingReply.Data) > 0 {
		queryEmbedding = embeddingReply.Data[0].Embedding
	}
	activeMemories, _ := u.store.ListRelevantMemories(ctx, userID, queryEmbedding, 5)
	modelRequest := &modelv1.ChatCompletionRequest{Messages: BuildChatMessagesForLocaleWithStage(history, activeMemories, conversation.Summary, content, nil, conversation.OnboardingStage, lexicon.LocaleFromContext(ctx)), MaxTokens: companionMaxTokens}
	response, err := u.models.Chat(ctx, modelRequest)
	if err != nil {
		return userMessage, nil, fmt.Errorf("generate companion response: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return userMessage, nil, fmt.Errorf("companion response is empty")
	}
	assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", response.Content)
	if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
		return userMessage, nil, fmt.Errorf("save assistant message: %w", err)
	}
	if err := u.advanceOnboarding(ctx, conversation, userID); err != nil {
		return userMessage, assistantMessage, err
	}
	u.enqueueMemory(memory.Job{UserID: userID, SourceMessageID: userMessage.MessageID, Content: content})
	return userMessage, assistantMessage, nil
}

func (u *CompanionUsecase) SendAudioMessage(ctx context.Context, req *v1.SendAudioMessageRequest) (*data.MessageModel, *data.MessageModel, []byte, string, error) {
	if req == nil || len(req.AudioData) == 0 {
		return nil, nil, nil, "", fmt.Errorf("audio data is required")
	}
	transcription, err := u.models.TranscribeAudio(ctx, &modelv1.TranscribeAudioRequest{AudioData: req.AudioData, Filename: req.Filename, ContentType: req.ContentType, Language: req.Language})
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("transcribe audio: %w", err)
	}
	content := strings.TrimSpace(transcription.Text)
	if content == "" {
		return nil, nil, nil, "", fmt.Errorf("transcription returned empty text")
	}
	userMessage, assistantMessage, err := u.SendMessage(ctx, &v1.SendMessageRequest{Content: content})
	if err != nil {
		return userMessage, assistantMessage, nil, "", err
	}
	if !req.Synthesize {
		return userMessage, assistantMessage, nil, "", nil
	}
	speech, err := u.models.SynthesizeSpeech(ctx, &modelv1.SynthesizeSpeechRequest{Text: assistantMessage.Content, Voice: req.Voice})
	if err != nil {
		return userMessage, assistantMessage, nil, "", fmt.Errorf("synthesize speech: %w", err)
	}
	return userMessage, assistantMessage, speech.AudioData, speech.ContentType, nil
}

func (u *CompanionUsecase) SendMediaMessage(ctx context.Context, req *v1.SendMediaMessageRequest) (*data.MessageModel, *data.MessageModel, error) {
	if req == nil || len(req.Data) == 0 {
		return nil, nil, fmt.Errorf("media data is required")
	}
	mediaType := strings.ToLower(strings.TrimSpace(req.MediaType))
	contentType := strings.ToLower(strings.TrimSpace(req.ContentType))
	if mediaType != "image" && mediaType != "video" {
		return nil, nil, fmt.Errorf("media type must be image or video")
	}
	if !strings.HasPrefix(contentType, mediaType+"/") {
		return nil, nil, fmt.Errorf("content type does not match media type")
	}
	maxBytes := 20 * 1024 * 1024
	if mediaType == "video" {
		maxBytes = 100 * 1024 * 1024
	}
	if len(req.Data) > maxBytes {
		return nil, nil, fmt.Errorf("media data exceeds %d bytes", maxBytes)
	}
	if u.assets == nil {
		return nil, nil, fmt.Errorf("asset storage is not configured")
	}
	conversation, userID, err := u.activeConversation(ctx)
	if err != nil {
		return nil, nil, err
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 100)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversation history: %w", err)
	}
	conversation, history, err = u.rollContextIfNeeded(ctx, conversation, history)
	if err != nil {
		return nil, nil, err
	}
	upload, err := u.assets.Upload(ctx, &assetv1.UploadFileRequest{Filename: strings.TrimSpace(req.Filename), ContentType: contentType, Data: req.Data, Metadata: map[string]string{"user_id": userID, "source": "companion-message", "media_type": mediaType}})
	if err != nil {
		return nil, nil, fmt.Errorf("upload companion media: %w", err)
	}
	if upload == nil || upload.FileId == "" || strings.TrimSpace(upload.Url) == "" {
		return nil, nil, fmt.Errorf("asset service returned incomplete upload")
	}
	caption := strings.TrimSpace(req.Caption)
	if caption == "" {
		caption = "[" + mediaType + "]"
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", caption)
	userMessage.Modality = mediaType
	uploadContentType := strings.TrimSpace(upload.ContentType)
	if uploadContentType == "" {
		uploadContentType = contentType
	}
	filename := upload.Name
	if filename == "" {
		filename = req.Filename
	}
	asset := data.MessageAssetModel{AssetID: upload.FileId, MediaType: mediaType, ContentType: uploadContentType, Filename: filename, URL: upload.Url, SizeBytes: upload.Size}
	userMessage.Assets = []data.MessageAssetModel{asset}
	creator, ok := u.store.(interface {
		CreateMessageWithAssets(context.Context, *data.MessageModel, []data.MessageAssetModel) error
	})
	if !ok {
		return nil, nil, fmt.Errorf("media message storage is not configured")
	}
	if err := creator.CreateMessageWithAssets(ctx, userMessage, userMessage.Assets); err != nil {
		return nil, nil, fmt.Errorf("save media message: %w", err)
	}
	if level := safety.CheckForLocale(caption, lexicon.LocaleFromContext(ctx)); level == safety.LevelCrisis {
		assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", safety.ResponseForLocale(level, lexicon.LocaleFromContext(ctx)))
		if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
			return userMessage, nil, fmt.Errorf("save safety response: %w", err)
		}
		return userMessage, assistantMessage, nil
	}
	activeMemories, _ := u.store.ListRelevantMemories(ctx, userID, nil, 5)
	modelRequest := &modelv1.ChatCompletionRequest{Messages: BuildChatMessagesForLocaleWithStage(history, activeMemories, conversation.Summary, caption, userMessage.Assets, conversation.OnboardingStage, lexicon.LocaleFromContext(ctx)), MaxTokens: companionMaxTokens}
	response, err := u.models.Chat(ctx, modelRequest)
	if err != nil {
		return userMessage, nil, fmt.Errorf("generate companion media response: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return userMessage, nil, fmt.Errorf("companion media response is empty")
	}
	assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", response.Content)
	if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
		return userMessage, nil, fmt.Errorf("save assistant message: %w", err)
	}
	if err := u.advanceOnboarding(ctx, conversation, userID); err != nil {
		return userMessage, assistantMessage, err
	}
	if caption != "["+mediaType+"]" {
		u.enqueueMemory(memory.Job{UserID: userID, SourceMessageID: userMessage.MessageID, Content: caption})
	}
	return userMessage, assistantMessage, nil
}

func (u *CompanionUsecase) SendMessageStream(ctx context.Context, req *v1.SendMessageRequest, emit func(*v1.MessageChunk) error) error {
	content := ""
	if req != nil {
		content = strings.TrimSpace(req.Content)
	}
	if content == "" {
		return fmt.Errorf("user identity and content are required")
	}
	conversation, userID, err := u.activeConversation(ctx)
	if err != nil {
		return err
	}
	if len(content) > maxMessageContentLength {
		return fmt.Errorf("message content exceeds %d bytes", maxMessageContentLength)
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 100)
	if err != nil {
		return fmt.Errorf("list conversation history: %w", err)
	}
	conversation, history, err = u.rollContextIfNeeded(ctx, conversation, history)
	if err != nil {
		return err
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", content)
	if err := u.store.CreateMessage(ctx, userMessage); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	if level := safety.CheckForLocale(content, lexicon.LocaleFromContext(ctx)); level == safety.LevelCrisis {
		assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", safety.ResponseForLocale(level, lexicon.LocaleFromContext(ctx)))
		if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
			return fmt.Errorf("save safety response: %w", err)
		}
		return emit(&v1.MessageChunk{MessageId: assistantMessage.MessageID, Delta: assistantMessage.Content, FinishReason: "safety", Done: true})
	}
	embeddingReply, embeddingErr := u.models.Embed(ctx, []string{content})
	var queryEmbedding []float32
	if embeddingErr == nil && embeddingReply != nil && len(embeddingReply.Data) > 0 {
		queryEmbedding = embeddingReply.Data[0].Embedding
	}
	activeMemories, _ := u.store.ListRelevantMemories(ctx, userID, queryEmbedding, 5)
	stream, err := u.models.ChatStream(ctx, &modelv1.ChatCompletionRequest{Messages: BuildChatMessagesForLocaleWithStage(history, activeMemories, conversation.Summary, content, nil, conversation.OnboardingStage, lexicon.LocaleFromContext(ctx)), MaxTokens: companionMaxTokens, Stream: true})
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("start companion stream: %w", err)
	}
	defer stream.CloseSend()
	assistantMessageID := "msg_" + uuid.NewString()
	var contentBuilder strings.Builder
	for {
		chunk, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("receive companion stream: %w", recvErr)
		}
		contentBuilder.WriteString(chunk.Delta)
		if err := emit(&v1.MessageChunk{MessageId: assistantMessageID, Delta: chunk.Delta, FinishReason: chunk.FinishReason, Done: chunk.Done}); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if chunk.Done {
			break
		}
	}
	assistantContent := strings.TrimSpace(contentBuilder.String())
	if assistantContent == "" {
		return fmt.Errorf("companion stream returned empty content")
	}
	assistantMessage := &data.MessageModel{MessageID: assistantMessageID, ConversationID: conversation.ConversationID, UserID: userID, Role: "assistant", Content: assistantContent, CreatedAt: time.Now().UTC()}
	if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
		return fmt.Errorf("save assistant message: %w", err)
	}
	if err := u.advanceOnboarding(ctx, conversation, userID); err != nil {
		return err
	}
	u.enqueueMemory(memory.Job{UserID: userID, SourceMessageID: userMessage.MessageID, Content: content})
	return nil
}

func (u *CompanionUsecase) advanceOnboarding(ctx context.Context, conversation *data.ConversationModel, userID string) error {
	if conversation == nil || normalizeOnboardingStage(conversation.OnboardingStage) == OnboardingStageEstablished {
		return nil
	}
	stage := nextOnboardingStage(conversation.OnboardingStage)
	if err := u.store.AdvanceOnboardingStage(ctx, conversation.ConversationID, userID, stage); err != nil {
		return fmt.Errorf("advance onboarding stage: %w", err)
	}
	conversation.OnboardingStage = stage
	return nil
}

func (u *CompanionUsecase) SubmitMemoryFeedback(ctx context.Context, req *v1.MemoryFeedbackRequest) error {
	userID := user_id.GetUserIDFromContext(ctx)
	if req == nil || userID == "" || strings.TrimSpace(req.MessageId) == "" {
		return fmt.Errorf("message_id and user identity are required")
	}
	messageID := strings.TrimSpace(req.MessageId)
	if _, err := u.store.GetMessage(ctx, messageID, userID); err != nil {
		return err
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "forget" && action != "do_not_remember" && action != "correct" {
		return fmt.Errorf("unsupported memory feedback action")
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	content := strings.TrimSpace(req.Content)
	if action == "correct" && (!memory.IsSupportedKind(kind) || content == "" || len(content) > 512) {
		return fmt.Errorf("corrected memory kind and content are required")
	}
	if err := u.store.DeleteMemoriesBySource(ctx, userID, messageID); err != nil {
		return fmt.Errorf("delete source memories: %w", err)
	}
	if action != "correct" {
		return nil
	}
	now := time.Now().UTC()
	return u.store.SaveMemory(ctx, &data.MemoryModel{
		MemoryID:        "mem_" + uuid.NewString(),
		UserID:          userID,
		Layer:           "L1",
		Kind:            kind,
		Content:         content,
		SourceMessageID: messageID,
		Confidence:      1,
		Importance:      3,
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
}

func MessageToProto(model *data.MessageModel) *v1.ConversationMessage {
	message := &v1.ConversationMessage{MessageId: model.MessageID, Role: model.Role, Content: model.Content, CreatedAt: model.CreatedAt.Format(time.RFC3339)}
	for _, asset := range model.Assets {
		message.Assets = append(message.Assets, &v1.MessageAsset{MessageAssetId: asset.MessageAssetID, AssetId: asset.AssetID, MediaType: asset.MediaType, ContentType: asset.ContentType, Filename: asset.Filename, Url: asset.URL, SizeBytes: asset.SizeBytes})
	}
	return message
}
