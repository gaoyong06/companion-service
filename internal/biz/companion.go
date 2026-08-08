package biz

import (
	"context"
	"fmt"
	"time"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/client"
	"companion-service/internal/data"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type CompanionUsecase struct {
	store  *data.Store
	models *client.ModelGatewayClient
}

func NewCompanionUsecase(store *data.Store, models *client.ModelGatewayClient) *CompanionUsecase {
	return &CompanionUsecase{store: store, models: models}
}

func (u *CompanionUsecase) CreateConversation(ctx context.Context, req *v1.CreateConversationRequest) (*data.ConversationModel, error) {
	userID := user_id.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("user identity is required")
	}
	companionID := req.CompanionId
	if companionID == "" {
		companionID = "default"
	}
	conversation := u.store.NewConversation(userID, companionID)
	if err := u.store.CreateConversation(ctx, conversation); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conversation, nil
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
	if req == nil || req.ConversationId == "" || userID == "" || req.Content == "" {
		return nil, nil, fmt.Errorf("conversation_id, user identity and content are required")
	}
	conversation, err := u.store.GetConversation(ctx, req.ConversationId, userID)
	if err != nil {
		return nil, nil, err
	}
	history, err := u.store.ListMessages(ctx, conversation.ConversationID, userID, 20)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversation history: %w", err)
	}
	userMessage := data.NewMessage(conversation.ConversationID, userID, "user", req.Content)
	if err := u.store.CreateMessage(ctx, userMessage); err != nil {
		return nil, nil, fmt.Errorf("save user message: %w", err)
	}
	modelRequest := &modelv1.ChatCompletionRequest{Messages: make([]*modelv1.ChatMessage, 0, len(history)+1)}
	for _, item := range history {
		modelRequest.Messages = append(modelRequest.Messages, &modelv1.ChatMessage{Role: item.Role, Content: item.Content})
	}
	modelRequest.Messages = append(modelRequest.Messages, &modelv1.ChatMessage{Role: "user", Content: req.Content})
	response, err := u.models.Chat(ctx, modelRequest)
	if err != nil {
		return userMessage, nil, fmt.Errorf("generate companion response: %w", err)
	}
	assistantMessage := data.NewMessage(conversation.ConversationID, userID, "assistant", response.Content)
	if err := u.store.CreateMessage(ctx, assistantMessage); err != nil {
		return userMessage, nil, fmt.Errorf("save assistant message: %w", err)
	}
	return userMessage, assistantMessage, nil
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
