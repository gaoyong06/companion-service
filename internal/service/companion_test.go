package service

import (
	"context"
	"io"
	"testing"

	assetv1 "asset-service/api/asset/v1"
	v1 "companion-service/api/companion/v1"
	"companion-service/internal/biz"
	"companion-service/internal/client"
	"companion-service/internal/data"
	"companion-service/internal/memory"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/log"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type serviceAssetStorage struct{}

func (serviceAssetStorage) Upload(context.Context, *assetv1.UploadFileRequest) (*assetv1.UploadFileReply, error) {
	return &assetv1.UploadFileReply{FileId: "asset-1", Name: "photo.png", ContentType: "image/png", Url: "https://asset.test/photo.png", Size: 4}, nil
}

type serviceStore struct {
	conversation  data.ConversationModel
	messages      []data.MessageModel
	advancedStage string
}

func (s *serviceStore) GetOrCreateActiveConversation(context.Context, string, string) (*data.ConversationModel, error) {
	return &s.conversation, nil
}
func (s *serviceStore) ListMessages(context.Context, string, string, int) ([]data.MessageModel, error) {
	return s.messages, nil
}
func (s *serviceStore) ListTimelineMessages(context.Context, string, int) ([]data.MessageModel, error) {
	return s.messages, nil
}
func (s *serviceStore) GetMessage(context.Context, string, string) (*data.MessageModel, error) {
	if len(s.messages) == 0 {
		return nil, data.ErrNotFound
	}
	message := s.messages[0]
	return &message, nil
}
func (s *serviceStore) CreateMessage(_ context.Context, message *data.MessageModel) error {
	s.messages = append(s.messages, *message)
	return nil
}
func (s *serviceStore) CreateMessageWithAssets(_ context.Context, message *data.MessageModel, assets []data.MessageAssetModel) error {
	message.Assets = append([]data.MessageAssetModel(nil), assets...)
	s.messages = append(s.messages, *message)
	return nil
}
func (s *serviceStore) ListRelevantMemories(context.Context, string, []float32, int) ([]data.MemoryModel, error) {
	return nil, nil
}
func (s *serviceStore) DeleteMemoriesBySource(context.Context, string, string) error { return nil }
func (s *serviceStore) SaveMemory(context.Context, *data.MemoryModel) error          { return nil }
func (s *serviceStore) AdvanceOnboardingStage(_ context.Context, conversationID, userID, stage string) error {
	if s.conversation.ConversationID != conversationID || s.conversation.UserID != userID {
		return data.ErrNotFound
	}
	s.advancedStage = stage
	s.conversation.OnboardingStage = stage
	return nil
}

type serviceModels struct{}

func (serviceModels) Chat(context.Context, *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error) {
	return &modelv1.ChatCompletionReply{Content: "I hear you."}, nil
}
func (serviceModels) ChatStream(context.Context, *modelv1.ChatCompletionRequest) (client.ChatStream, error) {
	return &serviceChatStream{}, nil
}
func (serviceModels) Embed(context.Context, []string) (*modelv1.CreateEmbeddingReply, error) {
	return &modelv1.CreateEmbeddingReply{}, nil
}
func (serviceModels) TranscribeAudio(context.Context, *modelv1.TranscribeAudioRequest) (*modelv1.TranscribeAudioReply, error) {
	return &modelv1.TranscribeAudioReply{Text: "hello"}, nil
}
func (serviceModels) SynthesizeSpeech(context.Context, *modelv1.SynthesizeSpeechRequest) (*modelv1.SynthesizeSpeechReply, error) {
	return &modelv1.SynthesizeSpeechReply{AudioData: []byte("audio"), ContentType: "audio/mpeg"}, nil
}

type serviceChatStream struct{ sent bool }

func (s *serviceChatStream) Recv() (*modelv1.ChatCompletionChunk, error) {
	if s.sent {
		return nil, io.EOF
	}
	s.sent = true
	return &modelv1.ChatCompletionChunk{Delta: "streamed", Done: true}, nil
}
func (*serviceChatStream) CloseSend() error { return nil }

type serviceMemoryProcessor struct{ forgotten string }

func (*serviceMemoryProcessor) Enqueue(memory.Job) error   { return nil }
func (p *serviceMemoryProcessor) ForgetUser(userID string) { p.forgotten = userID }

func newServiceFixture() (*CompanionService, *serviceStore, *serviceMemoryProcessor) {
	store := &serviceStore{conversation: data.ConversationModel{ConversationID: "conv-1", UserID: "user-1", CompanionID: "nana", Status: "active"}, messages: []data.MessageModel{{MessageID: "msg-1", ConversationID: "conv-1", UserID: "user-1", Role: "user", Content: "old"}}}
	processor := &serviceMemoryProcessor{}
	usecase := biz.NewCompanionUsecase(store, serviceModels{}, serviceAssetStorage{}, processor, log.NewStdLogger(io.Discard))
	return NewCompanionService(usecase), store, processor
}

func TestCompanionServiceWrapsUsecaseResponses(t *testing.T) {
	application, _, _ := newServiceFixture()
	ctx := user_id.WithUserID(context.Background(), "user-1")
	timeline, err := application.GetTimeline(ctx, &v1.GetTimelineRequest{})
	if err != nil || len(timeline.Messages) != 1 {
		t.Fatalf("timeline: %+v %v", timeline, err)
	}
	sent, err := application.SendMessage(ctx, &v1.SendMessageRequest{Content: "hello"})
	if err != nil || sent.AssistantMessage.Content != "I hear you." {
		t.Fatalf("send: %+v %v", sent, err)
	}
	streamChunks := make([]*v1.MessageChunk, 0, 1)
	if err := application.SendMessageStreamWithEmitter(ctx, &v1.SendMessageRequest{Content: "hello"}, func(chunk *v1.MessageChunk) error {
		streamChunks = append(streamChunks, chunk)
		return nil
	}); err != nil || len(streamChunks) != 1 {
		t.Fatalf("stream: %+v %v", streamChunks, err)
	}
	audio, err := application.SendAudioMessage(ctx, &v1.SendAudioMessageRequest{AudioData: []byte("raw"), Synthesize: true})
	if err != nil || string(audio.AudioData) != "audio" {
		t.Fatalf("audio: %+v %v", audio, err)
	}
	if _, err := application.SubmitMemoryFeedback(ctx, &v1.MemoryFeedbackRequest{MessageId: "msg-1", Action: "forget"}); err != nil {
		t.Fatalf("feedback: %v", err)
	}
}

func TestCompanionServiceWrapsMediaResponse(t *testing.T) {
	application, _, _ := newServiceFixture()
	ctx := user_id.WithUserID(context.Background(), "user-1")
	reply, err := application.SendMediaMessage(ctx, &v1.SendMediaMessageRequest{Data: []byte("image"), Filename: "photo.png", ContentType: "image/png", MediaType: "image"})
	if err != nil || reply == nil || reply.Message == nil || len(reply.Message.UserMessage.Assets) != 1 {
		t.Fatalf("media response: %+v %v", reply, err)
	}
}
