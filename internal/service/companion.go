package service

import (
	"context"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/biz"
)

type CompanionService struct {
	v1.UnimplementedCompanionServer
	usecase *biz.CompanionUsecase
}

func NewCompanionService(usecase *biz.CompanionUsecase) *CompanionService {
	return &CompanionService{usecase: usecase}
}

func (s *CompanionService) CreateConversation(ctx context.Context, req *v1.CreateConversationRequest) (*v1.ConversationReply, error) {
	conversation, err := s.usecase.CreateConversation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.ConversationReply{Conversation: biz.ConversationToProto(conversation, nil)}, nil
}

func (s *CompanionService) GetConversation(ctx context.Context, req *v1.GetConversationRequest) (*v1.ConversationReply, error) {
	conversation, messages, err := s.usecase.GetConversation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.ConversationReply{Conversation: biz.ConversationToProto(conversation, messages)}, nil
}

func (s *CompanionService) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*v1.SendMessageReply, error) {
	userMessage, assistantMessage, err := s.usecase.SendMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.SendMessageReply{UserMessage: biz.MessageToProto(userMessage), AssistantMessage: biz.MessageToProto(assistantMessage)}, nil
}
