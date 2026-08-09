package biz

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/client"
	"companion-service/internal/data"
	"companion-service/internal/memory"
	"companion-service/internal/safety"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/google/uuid"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type CompanionUsecase struct {
	store  *data.Store
	models *client.ModelGatewayClient
	memory *memory.Processor
}

const maxMessageContentLength = 16 * 1024

const companionMaxTokens int32 = 256

const summaryPrompt = "Summarize this conversation in concise Chinese. Keep only durable context, decisions, ongoing goals, and unresolved topics. Do not include secrets or sensitive personal data. Return plain text under 2000 characters."

func NewCompanionUsecase(store *data.Store, models *client.ModelGatewayClient, processor *memory.Processor) *CompanionUsecase {
	return &CompanionUsecase{store: store, models: models, memory: processor}
}

func (u *CompanionUsecase) CreateConversation(ctx context.Context, req *v1.CreateConversationRequest) (*data.ConversationModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user identity is required")
	}
	companionID := ""
	if req != nil {
		companionID = strings.TrimSpace(req.CompanionId)
	}
	if companionID == "" {
		companionID = "default"
	}
	conversation := u.store.NewConversation(userID, companionID)
	if err := u.store.CreateConversation(ctx, conversation); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conversation, nil
}

func (u *CompanionUsecase) ListConversations(ctx context.Context, req *v1.ListConversationsRequest) ([]data.ConversationModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user identity is required")
	}
	limit := 20
	if req != nil && req.Limit > 0 {
		limit = int(req.Limit)
	}
	return u.store.ListConversations(ctx, userID, limit)
}

func (u *CompanionUsecase) GetConversation(ctx context.Context, req *v1.GetConversationRequest) (*data.ConversationModel, []data.MessageModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if req == nil || req.ConversationId == "" || userID == "" {
		return nil, nil, fmt.Errorf("conversation_id and user identity are required")
	}
	conversation, err := u.store.GetConversation(ctx, req.ConversationId, userID)
	if err != nil {
		return nil, nil, err
	}
	messages, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 50)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversation messages: %w", err)
	}
	return conversation, messages, nil
}

func (u *CompanionUsecase) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*data.MessageModel, *data.MessageModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	content := ""
	conversationID := ""
	if req != nil {
		content = strings.TrimSpace(req.Content)
		conversationID = strings.TrimSpace(req.ConversationId)
	}
	if conversationID == "" || userID == "" || content == "" {
		return nil, nil, fmt.Errorf("conversation_id, user identity and content are required")
	}
	if len(content) > maxMessageContentLength {
		return nil, nil, fmt.Errorf("message content exceeds %d bytes", maxMessageContentLength)
	}
	conversation, err := u.store.GetConversation(ctx, conversationID, userID)
	if err != nil {
		return nil, nil, err
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 20)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversation history: %w", err)
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", content)
	if err := u.store.CreateMessage(ctx, userMessage); err != nil {
		return nil, nil, fmt.Errorf("save user message: %w", err)
	}
	if level := safety.Check(content); level == safety.LevelCrisis {
		assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", safety.Response(level))
		if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
			return userMessage, nil, fmt.Errorf("save safety response: %w", err)
		}
		return userMessage, assistantMessage, nil
	}
	embeddingReply, embeddingErr := u.models.Embed(ctx, []string{content})
	var queryEmbedding []float32
	if embeddingErr == nil && len(embeddingReply.Data) > 0 {
		queryEmbedding = embeddingReply.Data[0].Embedding
	}
	activeMemories, _ := u.store.ListRelevantMemories(ctx, userID, queryEmbedding, 5)
	modelRequest := &modelv1.ChatCompletionRequest{Messages: BuildChatMessages(history, activeMemories, content), MaxTokens: companionMaxTokens}
	response, err := u.models.Chat(ctx, modelRequest)
	if err != nil {
		return userMessage, nil, fmt.Errorf("generate companion response: %w", err)
	}
	assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", response.Content)
	if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
		return userMessage, nil, fmt.Errorf("save assistant message: %w", err)
	}
	if u.memory != nil {
		_ = u.memory.Enqueue(memory.Job{UserID: userID, SourceMessageID: userMessage.MessageID, Content: content})
	}
	return userMessage, assistantMessage, nil
}

func (u *CompanionUsecase) SendAudioMessage(ctx context.Context, req *v1.SendAudioMessageRequest) (*data.MessageModel, *data.MessageModel, []byte, string, error) {
	if req == nil || len(req.AudioData) == 0 || strings.TrimSpace(req.ConversationId) == "" {
		return nil, nil, nil, "", fmt.Errorf("conversation_id and audio data are required")
	}
	transcription, err := u.models.TranscribeAudio(ctx, &modelv1.TranscribeAudioRequest{AudioData: req.AudioData, Filename: req.Filename, ContentType: req.ContentType, Language: req.Language})
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("transcribe audio: %w", err)
	}
	content := strings.TrimSpace(transcription.Text)
	if content == "" {
		return nil, nil, nil, "", fmt.Errorf("transcription returned empty text")
	}
	userMessage, assistantMessage, err := u.SendMessage(ctx, &v1.SendMessageRequest{ConversationId: req.ConversationId, Content: content})
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

func (u *CompanionUsecase) SendMessageStream(ctx context.Context, req *v1.SendMessageRequest, emit func(*v1.MessageChunk) error) error {
	userID := user_id.GetUserIDFromContext(ctx)
	content := ""
	conversationID := ""
	if req != nil {
		content = strings.TrimSpace(req.Content)
		conversationID = strings.TrimSpace(req.ConversationId)
	}
	if conversationID == "" || userID == "" || content == "" {
		return fmt.Errorf("conversation_id, user identity and content are required")
	}
	if len(content) > maxMessageContentLength {
		return fmt.Errorf("message content exceeds %d bytes", maxMessageContentLength)
	}
	conversation, err := u.store.GetConversation(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 20)
	if err != nil {
		return fmt.Errorf("list conversation history: %w", err)
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", content)
	if err := u.store.CreateMessage(ctx, userMessage); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	if level := safety.Check(content); level == safety.LevelCrisis {
		assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", safety.Response(level))
		if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
			return fmt.Errorf("save safety response: %w", err)
		}
		return emit(&v1.MessageChunk{MessageId: assistantMessage.MessageID, Delta: assistantMessage.Content, FinishReason: "safety", Done: true})
	}
	embeddingReply, embeddingErr := u.models.Embed(ctx, []string{content})
	var queryEmbedding []float32
	if embeddingErr == nil && len(embeddingReply.Data) > 0 {
		queryEmbedding = embeddingReply.Data[0].Embedding
	}
	activeMemories, _ := u.store.ListRelevantMemories(ctx, userID, queryEmbedding, 5)
	stream, err := u.models.ChatStream(ctx, &modelv1.ChatCompletionRequest{Messages: BuildChatMessages(history, activeMemories, content), MaxTokens: companionMaxTokens, Stream: true})
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
	if u.memory != nil {
		_ = u.memory.Enqueue(memory.Job{UserID: userID, SourceMessageID: userMessage.MessageID, Content: content})
	}
	return nil
}

func (u *CompanionUsecase) SubmitMemoryFeedback(ctx context.Context, req *v1.MemoryFeedbackRequest) error {
	userID := user_id.GetUserIDFromContext(ctx)
	if req == nil || userID == "" || strings.TrimSpace(req.ConversationId) == "" || strings.TrimSpace(req.MessageId) == "" {
		return fmt.Errorf("conversation_id, message_id and user identity are required")
	}
	conversationID := strings.TrimSpace(req.ConversationId)
	messageID := strings.TrimSpace(req.MessageId)
	if _, err := u.store.GetConversation(ctx, conversationID, userID); err != nil {
		return err
	}
	if _, err := u.store.GetMessage(ctx, messageID, conversationID, userID); err != nil {
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

func (u *CompanionUsecase) CloseConversation(ctx context.Context, req *v1.CloseConversationRequest) (*data.ConversationModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if req == nil || userID == "" || strings.TrimSpace(req.ConversationId) == "" {
		return nil, fmt.Errorf("conversation_id and user identity are required")
	}
	conversationID := strings.TrimSpace(req.ConversationId)
	conversation, err := u.store.GetConversation(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	if conversation.Status != "active" {
		return conversation, nil
	}
	history, err := u.store.ListMessages(ctx, conversationID, userID, 50)
	if err != nil {
		return nil, fmt.Errorf("list conversation messages: %w", err)
	}
	summary := strings.TrimSpace(conversation.Summary)
	if len(history) > 0 {
		messages := []*modelv1.ChatMessage{{Role: "system", Content: summaryPrompt}}
		messages = append(messages, BuildChatMessages(history, nil, "")...)
		response, callErr := u.models.Chat(ctx, &modelv1.ChatCompletionRequest{Messages: messages})
		if callErr != nil {
			return nil, fmt.Errorf("generate conversation summary: %w", callErr)
		}
		summary = strings.TrimSpace(response.Content)
		if runes := []rune(summary); len(runes) > 2000 {
			summary = string(runes[:2000])
		}
	}
	closed, err := u.store.CloseConversation(ctx, conversationID, userID, summary, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("close conversation: %w", err)
	}
	return closed, nil
}

func (u *CompanionUsecase) ExportData(ctx context.Context) (*v1.ExportDataReply, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user identity is required")
	}
	conversations, messages, memories, err := u.store.ExportUserData(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export user data: %w", err)
	}
	messageGroups := make(map[string][]data.MessageModel, len(conversations))
	for _, message := range messages {
		messageGroups[message.ConversationID] = append(messageGroups[message.ConversationID], message)
	}
	reply := &v1.ExportDataReply{Conversations: make([]*v1.Conversation, 0, len(conversations)), Memories: make([]*v1.MemorySnapshot, 0, len(memories))}
	for index := range conversations {
		conversation := conversations[index]
		reply.Conversations = append(reply.Conversations, ConversationToProto(&conversation, messageGroups[conversation.ConversationID]))
	}
	for _, memoryModel := range memories {
		reply.Memories = append(reply.Memories, &v1.MemorySnapshot{MemoryId: memoryModel.MemoryID, Layer: memoryModel.Layer, Kind: memoryModel.Kind, Content: memoryModel.Content, Status: memoryModel.Status, CreatedAt: memoryModel.CreatedAt.Format(time.RFC3339), UpdatedAt: memoryModel.UpdatedAt.Format(time.RFC3339)})
	}
	return reply, nil
}

func (u *CompanionUsecase) DeleteData(ctx context.Context) error {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return fmt.Errorf("user identity is required")
	}
	if u.memory != nil {
		u.memory.ForgetUser(userID)
	}
	if err := u.store.DeleteUserData(ctx, userID); err != nil {
		return fmt.Errorf("delete user data: %w", err)
	}
	return nil
}

func ConversationsToProto(models []data.ConversationModel) []*v1.Conversation {
	result := make([]*v1.Conversation, 0, len(models))
	for index := range models {
		result = append(result, ConversationToProto(&models[index], nil))
	}
	return result
}

func ConversationToProto(model *data.ConversationModel, messages []data.MessageModel) *v1.Conversation {
	result := &v1.Conversation{ConversationId: model.ConversationID, UserId: model.UserID, CompanionId: model.CompanionID, Status: model.Status, Summary: model.Summary, CreatedAt: model.CreatedAt.Format(time.RFC3339), UpdatedAt: model.UpdatedAt.Format(time.RFC3339)}
	for _, message := range messages {
		result.Messages = append(result.Messages, MessageToProto(&message))
	}
	return result
}

func MessageToProto(model *data.MessageModel) *v1.ConversationMessage {
	return &v1.ConversationMessage{MessageId: model.MessageID, Role: model.Role, Content: model.Content, CreatedAt: model.CreatedAt.Format(time.RFC3339)}
}
