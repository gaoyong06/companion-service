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

func (s *CompanionService) GetTimeline(ctx context.Context, req *v1.GetTimelineRequest) (*v1.TimelineReply, error) {
	messages, err := s.usecase.GetTimeline(ctx, req)
	if err != nil {
		return nil, err
	}
	reply := &v1.TimelineReply{Messages: make([]*v1.ConversationMessage, 0, len(messages))}
	for index := range messages {
		reply.Messages = append(reply.Messages, biz.MessageToProto(&messages[index]))
	}
	return reply, nil
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

func (s *CompanionService) SendMediaMessage(ctx context.Context, req *v1.SendMediaMessageRequest) (*v1.SendMediaMessageReply, error) {
	userMessage, assistantMessage, err := s.usecase.SendMediaMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &v1.SendMediaMessageReply{Message: &v1.SendMessageReply{UserMessage: biz.MessageToProto(userMessage), AssistantMessage: biz.MessageToProto(assistantMessage)}}, nil
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
