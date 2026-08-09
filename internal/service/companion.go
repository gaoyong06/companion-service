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

func (s *CompanionService) ListConversations(ctx context.Context, req *v1.ListConversationsRequest) (*v1.ConversationListReply, error) {
	conversations, err := s.usecase.ListConversations(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.ConversationListReply{Conversations: biz.ConversationsToProto(conversations)}, nil
}

func (s *CompanionService) SendMessage(ctx context.Context, req *v1.SendMessageRequest) (*v1.SendMessageReply, error) {
	userMessage, assistantMessage, err := s.usecase.SendMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.SendMessageReply{UserMessage: biz.MessageToProto(userMessage), AssistantMessage: biz.MessageToProto(assistantMessage)}, nil
}

func (s *CompanionService) SendAudioMessage(ctx context.Context, req *v1.SendAudioMessageRequest) (*v1.SendAudioMessageReply, error) {
	userMessage, assistantMessage, audioData, contentType, err := s.usecase.SendAudioMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.SendAudioMessageReply{Message: &v1.SendMessageReply{UserMessage: biz.MessageToProto(userMessage), AssistantMessage: biz.MessageToProto(assistantMessage)}, AudioData: audioData, AudioContentType: contentType}, nil
}

func (s *CompanionService) SendMessageStream(req *v1.SendMessageRequest, server v1.Companion_SendMessageStreamServer) error {
	return s.SendMessageStreamWithEmitter(server.Context(), req, server.Send)
}

func (s *CompanionService) SendMessageStreamWithEmitter(ctx context.Context, req *v1.SendMessageRequest, emit func(*v1.MessageChunk) error) error {
	return s.usecase.SendMessageStream(ctx, req, emit)
}

func (s *CompanionService) SubmitMemoryFeedback(ctx context.Context, req *v1.MemoryFeedbackRequest) (*v1.MemoryFeedbackReply, error) {
	if err := s.usecase.SubmitMemoryFeedback(ctx, req); err != nil {
		return nil, err
	}
	return &v1.MemoryFeedbackReply{Accepted: true}, nil
}

func (s *CompanionService) CloseConversation(ctx context.Context, req *v1.CloseConversationRequest) (*v1.ConversationReply, error) {
	conversation, err := s.usecase.CloseConversation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.ConversationReply{Conversation: biz.ConversationToProto(conversation, nil)}, nil
}

func (s *CompanionService) ExportData(ctx context.Context, _ *v1.ExportDataRequest) (*v1.ExportDataReply, error) {
	return s.usecase.ExportData(ctx)
}

func (s *CompanionService) DeleteData(ctx context.Context, _ *v1.DeleteDataRequest) (*v1.DeleteDataReply, error) {
	if err := s.usecase.DeleteData(ctx); err != nil {
		return nil, err
	}
	return &v1.DeleteDataReply{Accepted: true}, nil
}
