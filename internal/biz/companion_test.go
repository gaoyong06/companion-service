package biz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	assetv1 "asset-service/api/asset/v1"
	v1 "companion-service/api/companion/v1"
	"companion-service/internal/client"
	"companion-service/internal/data"
	"companion-service/internal/memory"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/log"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type fakeConversationStore struct {
	conversation  *data.ConversationModel
	messages      []data.MessageModel
	memories      []data.MemoryModel
	created       []data.MessageModel
	feedback      []data.MemoryModel
	assets        []data.MessageAssetModel
	rolled        bool
	rolledSummary string
	advancedStage string
	forgotten     string
	err           error
}

func (s *fakeConversationStore) GetOrCreateActiveConversation(context.Context, string, string) (*data.ConversationModel, error) {
	if s.conversation == nil {
		return nil, data.ErrNotFound
	}
	copy := *s.conversation
	return &copy, s.err
}
func (s *fakeConversationStore) ListMessages(context.Context, string, string, int) ([]data.MessageModel, error) {
	return s.messages, s.err
}
func (s *fakeConversationStore) ListTimelineMessages(context.Context, string, int) ([]data.MessageModel, error) {
	return s.messages, s.err
}
func (s *fakeConversationStore) GetMessage(context.Context, string, string) (*data.MessageModel, error) {
	if len(s.messages) == 0 {
		return nil, data.ErrNotFound
	}
	message := s.messages[0]
	return &message, s.err
}
func (s *fakeConversationStore) CreateMessage(_ context.Context, message *data.MessageModel) error {
	if s.err != nil {
		return s.err
	}
	s.created = append(s.created, *message)
	return nil
}
func (s *fakeConversationStore) ListRelevantMemories(context.Context, string, []float32, int) ([]data.MemoryModel, error) {
	return s.memories, s.err
}
func (s *fakeConversationStore) DeleteMemoriesBySource(context.Context, string, string) error {
	return s.err
}
func (s *fakeConversationStore) SaveMemory(_ context.Context, model *data.MemoryModel) error {
	s.feedback = append(s.feedback, *model)
	return s.err
}
func (s *fakeConversationStore) CreateMessageWithAssets(_ context.Context, message *data.MessageModel, assets []data.MessageAssetModel) error {
	if s.err != nil {
		return s.err
	}
	message.Assets = append([]data.MessageAssetModel(nil), assets...)
	s.assets = append(s.assets, assets...)
	s.created = append(s.created, *message)
	return nil
}
func (s *fakeConversationStore) RollActiveConversation(_ context.Context, conversation *data.ConversationModel, summary string) (*data.ConversationModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.rolled = true
	s.rolledSummary = summary
	copy := *conversation
	copy.Summary = summary
	return &copy, nil
}

func (s *fakeConversationStore) AdvanceOnboardingStage(_ context.Context, conversationID, userID, stage string) error {
	if s.err != nil {
		return s.err
	}
	if s.conversation == nil || s.conversation.ConversationID != conversationID || s.conversation.UserID != userID {
		return data.ErrNotFound
	}
	s.advancedStage = stage
	s.conversation.OnboardingStage = stage
	return nil
}

type fakeAssetStorage struct {
	req   *assetv1.UploadFileRequest
	reply *assetv1.UploadFileReply
	err   error
}

func (s *fakeAssetStorage) Upload(_ context.Context, req *assetv1.UploadFileRequest) (*assetv1.UploadFileReply, error) {
	s.req = req
	return s.reply, s.err
}

type fakeMemoryProcessor struct {
	jobs       []memory.Job
	forgotten  string
	enqueueErr error
}

func (p *fakeMemoryProcessor) Enqueue(job memory.Job) error {
	p.jobs = append(p.jobs, job)
	return p.enqueueErr
}
func (p *fakeMemoryProcessor) ForgetUser(userID string) { p.forgotten = userID }

type captureLogger struct {
	entries []string
}

func (l *captureLogger) Log(_ log.Level, keyvals ...any) error {
	l.entries = append(l.entries, fmt.Sprint(keyvals...))
	return nil
}

type fakeChatStream struct {
	chunks []*modelv1.ChatCompletionChunk
	index  int
	err    error
	closed bool
}

func (s *fakeChatStream) Recv() (*modelv1.ChatCompletionChunk, error) {
	if s.err != nil {
		err := s.err
		s.err = nil
		return nil, err
	}
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}
func (s *fakeChatStream) CloseSend() error { s.closed = true; return nil }

type fakeModelGateway struct {
	chatReply        *modelv1.ChatCompletionReply
	embedding        *modelv1.CreateEmbeddingReply
	transcription    *modelv1.TranscribeAudioReply
	speech           *modelv1.SynthesizeSpeechReply
	stream           func() (client.ChatStream, error)
	chatErr          error
	embedErr         error
	transcriptionErr error
	speechErr        error
	chatCalls        int
	embedCalls       int
	lastChatRequest  *modelv1.ChatCompletionRequest
}

func (m *fakeModelGateway) Chat(_ context.Context, request *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error) {
	m.chatCalls++
	m.lastChatRequest = request
	return m.chatReply, m.chatErr
}
func (m *fakeModelGateway) ChatStream(context.Context, *modelv1.ChatCompletionRequest) (client.ChatStream, error) {
	if m.stream == nil {
		return nil, errors.New("stream is not configured")
	}
	return m.stream()
}
func (m *fakeModelGateway) Embed(context.Context, []string) (*modelv1.CreateEmbeddingReply, error) {
	m.embedCalls++
	return m.embedding, m.embedErr
}
func (m *fakeModelGateway) TranscribeAudio(context.Context, *modelv1.TranscribeAudioRequest) (*modelv1.TranscribeAudioReply, error) {
	return m.transcription, m.transcriptionErr
}
func (m *fakeModelGateway) SynthesizeSpeech(context.Context, *modelv1.SynthesizeSpeechRequest) (*modelv1.SynthesizeSpeechReply, error) {
	return m.speech, m.speechErr
}

// Compile-time assertion keeps the fake aligned with the production boundary.
var _ interface {
	Chat(context.Context, *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error)
	ChatStream(context.Context, *modelv1.ChatCompletionRequest) (client.ChatStream, error)
} = (*fakeModelGateway)(nil)

func companionContext() context.Context { return user_id.WithUserID(context.Background(), "user-1") }

func newUsecaseFixture() (*CompanionUsecase, *fakeConversationStore, *fakeModelGateway, *fakeMemoryProcessor) {
	store := &fakeConversationStore{conversation: &data.ConversationModel{ConversationID: "conv-1", UserID: "user-1", CompanionID: "nana", Status: "active"}, messages: []data.MessageModel{{MessageID: "msg-1", ConversationID: "conv-1", UserID: "user-1", Role: "user", Content: "old"}}}
	model := &fakeModelGateway{chatReply: &modelv1.ChatCompletionReply{Content: "I hear you."}, embedding: &modelv1.CreateEmbeddingReply{Data: []*modelv1.EmbeddingItem{{Embedding: []float32{0.1, 0.2}}}}, transcription: &modelv1.TranscribeAudioReply{Text: "hello"}, speech: &modelv1.SynthesizeSpeechReply{AudioData: []byte("audio"), ContentType: "audio/mpeg"}}
	memoryProcessor := &fakeMemoryProcessor{}
	return NewCompanionUsecase(store, model, nil, memoryProcessor, log.NewStdLogger(io.Discard)), store, model, memoryProcessor
}

func TestConversationUsecaseResolvesTimelineWithoutClientConversationID(t *testing.T) {
	usecase, store, _, _ := newUsecaseFixture()
	ctx := companionContext()
	messages, err := usecase.GetTimeline(ctx, &v1.GetTimelineRequest{Limit: 10})
	if err != nil || len(messages) != 1 || messages[0].MessageID != "msg-1" {
		t.Fatalf("unexpected timeline: %v %+v", err, messages)
	}
	if len(store.created) != 0 {
		t.Fatalf("read paths should not create messages: %+v", store.created)
	}
}

func TestCompanionUsecasePropagatesModelAndPersistenceErrors(t *testing.T) {
	usecase, store, model, _ := newUsecaseFixture()
	model.chatErr = errors.New("provider unavailable")
	if _, _, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "hello"}); err == nil || !strings.Contains(err.Error(), "generate companion response") {
		t.Fatalf("expected chat error, got %v", err)
	}
	model.chatErr = nil
	model.transcriptionErr = errors.New("stt unavailable")
	if _, _, _, _, err := usecase.SendAudioMessage(companionContext(), &v1.SendAudioMessageRequest{AudioData: []byte("voice")}); err == nil || !strings.Contains(err.Error(), "transcribe audio") {
		t.Fatalf("expected stt error, got %v", err)
	}
	store.err = errors.New("database unavailable")
	if _, _, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "hello"}); err == nil {
		t.Fatal("expected persistence error")
	}
}

func TestSendMessagePersistsReplyUsesEmbeddingAndEnqueuesMemory(t *testing.T) {
	usecase, store, model, processor := newUsecaseFixture()
	userMessage, assistantMessage, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "I like tea"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if userMessage.Role != "user" || assistantMessage.Content != "I hear you." || model.chatCalls != 1 || model.embedCalls != 1 {
		t.Fatalf("unexpected orchestration: %+v %+v calls=%d/%d", userMessage, assistantMessage, model.chatCalls, model.embedCalls)
	}
	if len(store.created) != 2 || len(processor.jobs) != 1 || processor.jobs[0].SourceMessageID != userMessage.MessageID {
		t.Fatalf("unexpected persistence/jobs: %+v %+v", store.created, processor.jobs)
	}
	if store.advancedStage != OnboardingStageSmallTalk {
		t.Fatalf("expected first successful reply to advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendMessageLogsMemoryEnqueueFailureWithoutFailingReply(t *testing.T) {
	store := &fakeConversationStore{conversation: &data.ConversationModel{ConversationID: "conv-1", UserID: "user-1", CompanionID: "nana", Status: "active"}}
	model := &fakeModelGateway{chatReply: &modelv1.ChatCompletionReply{Content: "I hear you."}, embedding: &modelv1.CreateEmbeddingReply{Data: []*modelv1.EmbeddingItem{{Embedding: []float32{0.1, 0.2}}}}}
	processor := &fakeMemoryProcessor{enqueueErr: errors.New("queue unavailable")}
	logger := &captureLogger{}
	usecase := NewCompanionUsecase(store, model, nil, processor, logger)
	_, _, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "I like tea"})
	if err != nil {
		t.Fatalf("memory queue failure must not fail the completed reply: %v", err)
	}
	if len(processor.jobs) != 1 {
		t.Fatalf("expected one memory job despite queue failure: %+v", processor.jobs)
	}
	if len(logger.entries) != 1 || !strings.Contains(logger.entries[0], "enqueue memory job failed") {
		t.Fatalf("expected enqueue failure log, entries=%v", logger.entries)
	}
}

func TestOnboardingStageTransitions(t *testing.T) {
	cases := []struct {
		name string
		from string
		want string
	}{
		{name: "first meeting", from: OnboardingStageFirstMeeting, want: OnboardingStageSmallTalk},
		{name: "small talk", from: OnboardingStageSmallTalk, want: OnboardingStageGettingToKnow},
		{name: "getting to know", from: OnboardingStageGettingToKnow, want: OnboardingStageTrust},
		{name: "trust", from: OnboardingStageTrust, want: OnboardingStageEstablished},
		{name: "established", from: OnboardingStageEstablished, want: OnboardingStageEstablished},
		{name: "unknown defaults to first meeting", from: "unknown", want: OnboardingStageSmallTalk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextOnboardingStage(tc.from); got != tc.want {
				t.Fatalf("next stage(%q) = %q, want %q", tc.from, got, tc.want)
			}
		})
	}
}

func TestSendMessageDoesNotAdvanceOnModelFailure(t *testing.T) {
	usecase, store, model, _ := newUsecaseFixture()
	model.chatErr = errors.New("provider unavailable")
	if _, _, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "hello"}); err == nil {
		t.Fatal("expected model failure")
	}
	if store.advancedStage != "" {
		t.Fatalf("model failure must not advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendMessageDoesNotAdvanceOnEmptyModelResponse(t *testing.T) {
	usecase, store, model, _ := newUsecaseFixture()
	model.chatReply = nil
	if _, _, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "hello"}); err == nil || !strings.Contains(err.Error(), "response is empty") {
		t.Fatalf("expected empty response error, got %v", err)
	}
	if store.advancedStage != "" {
		t.Fatalf("empty model response must not advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendMessageCrisisSkipsModelAndReturnsSafetyReply(t *testing.T) {
	usecase, store, model, processor := newUsecaseFixture()
	_, reply, err := usecase.SendMessage(companionContext(), &v1.SendMessageRequest{Content: "I want to die"})
	if err != nil || reply == nil || !strings.Contains(reply.Content, "急救") {
		t.Fatalf("unexpected safety reply: %v %+v", err, reply)
	}
	if model.chatCalls != 0 || model.embedCalls != 0 || len(processor.jobs) != 0 || len(store.created) != 2 {
		t.Fatalf("safety path called downstream: chat=%d embed=%d jobs=%d writes=%d", model.chatCalls, model.embedCalls, len(processor.jobs), len(store.created))
	}
	if store.advancedStage != "" {
		t.Fatalf("crisis response must not advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendMessageStreamPersistsCompletedAssistantAndEmitsChunks(t *testing.T) {
	usecase, store, _, processor := newUsecaseFixture()
	stream := &fakeChatStream{chunks: []*modelv1.ChatCompletionChunk{{Delta: "hi"}, {Delta: " there", Done: true, FinishReason: "stop"}}}
	usecase.models = &fakeModelGateway{embedding: &modelv1.CreateEmbeddingReply{}, stream: func() (client.ChatStream, error) { return stream, nil }}
	var chunks []*v1.MessageChunk
	err := usecase.SendMessageStream(companionContext(), &v1.SendMessageRequest{Content: "hello"}, func(chunk *v1.MessageChunk) error { chunks = append(chunks, chunk); return nil })
	if err != nil || len(chunks) != 2 || chunks[1].FinishReason != "stop" || !stream.closed {
		t.Fatalf("unexpected stream result: %v chunks=%+v closed=%v", err, chunks, stream.closed)
	}
	if len(store.created) != 2 || len(processor.jobs) != 1 {
		t.Fatalf("stream persistence missing: writes=%d jobs=%d", len(store.created), len(processor.jobs))
	}
	if store.advancedStage != OnboardingStageSmallTalk {
		t.Fatalf("expected stream reply to advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendMessageStreamDoesNotAdvanceOnModelFailure(t *testing.T) {
	usecase, store, _, _ := newUsecaseFixture()
	usecase.models = &fakeModelGateway{
		embedding: &modelv1.CreateEmbeddingReply{},
		stream:    func() (client.ChatStream, error) { return nil, errors.New("provider unavailable") },
	}
	if err := usecase.SendMessageStream(companionContext(), &v1.SendMessageRequest{Content: "hello"}, func(*v1.MessageChunk) error { return nil }); err == nil {
		t.Fatal("expected stream failure")
	}
	if store.advancedStage != "" {
		t.Fatalf("stream failure must not advance onboarding, got %q", store.advancedStage)
	}
}

func TestSendAudioMessageTranscribesAndSynthesizes(t *testing.T) {
	usecase, _, model, _ := newUsecaseFixture()
	userMessage, assistantMessage, audio, contentType, err := usecase.SendAudioMessage(companionContext(), &v1.SendAudioMessageRequest{AudioData: []byte("voice"), Synthesize: true})
	if err != nil || userMessage.Content != "hello" || assistantMessage.Content == "" || string(audio) != "audio" || contentType != "audio/mpeg" {
		t.Fatalf("unexpected audio flow: %v %+v %+v %q %s", err, userMessage, assistantMessage, audio, contentType)
	}
	if model.chatCalls != 1 {
		t.Fatalf("expected chat after transcription, got %d", model.chatCalls)
	}
}

func TestMemoryFeedbackCorrectAndRejectsInvalidAction(t *testing.T) {
	usecase, store, _, _ := newUsecaseFixture()
	ctx := companionContext()
	if err := usecase.SubmitMemoryFeedback(ctx, &v1.MemoryFeedbackRequest{MessageId: "msg-1", Action: "correct", Kind: "preference", Content: "likes tea"}); err != nil {
		t.Fatalf("correct feedback: %v", err)
	}
	if len(store.feedback) != 1 || store.feedback[0].Content != "likes tea" {
		t.Fatalf("unexpected corrected memory: %+v", store.feedback)
	}
	if err := usecase.SubmitMemoryFeedback(ctx, &v1.MemoryFeedbackRequest{MessageId: "msg-1", Action: "unknown"}); err == nil {
		t.Fatal("expected invalid action error")
	}
}

func TestMessageProtoConversionPreservesFields(t *testing.T) {
	now := time.Date(2026, 8, 9, 1, 2, 3, 0, time.UTC)
	message := data.MessageModel{MessageID: "msg-1", Role: "assistant", Content: "hello", CreatedAt: now}
	converted := MessageToProto(&message)
	if converted.MessageId != "msg-1" || converted.Content != "hello" || converted.CreatedAt != now.Format(time.RFC3339) {
		t.Fatalf("unexpected proto conversion: %+v", converted)
	}
}

func TestMessageToProtoPreservesMediaAssets(t *testing.T) {
	message := &data.MessageModel{MessageID: "msg-1", Role: "user", Content: "[image]", Modality: "image", Assets: []data.MessageAssetModel{{MessageAssetID: "ma-1", AssetID: "asset-1", MediaType: "image", ContentType: "image/png", Filename: "photo.png", URL: "https://asset.test/photo.png", SizeBytes: 12}}}
	converted := MessageToProto(message)
	if len(converted.Assets) != 1 || converted.Assets[0].AssetId != "asset-1" || converted.Assets[0].Url != "https://asset.test/photo.png" || converted.Assets[0].SizeBytes != 12 {
		t.Fatalf("media asset conversion lost fields: %+v", converted)
	}
}

func TestSendMediaMessageUploadsPersistsAndBuildsMultimodalRequest(t *testing.T) {
	usecase, store, model, processor := newUsecaseFixture()
	assets := &fakeAssetStorage{reply: &assetv1.UploadFileReply{FileId: "asset-1", Name: "photo.png", ContentType: "image/png", Url: "https://asset.test/photo.png", Size: 4}}
	usecase.assets = assets
	userMessage, assistantMessage, err := usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: []byte("data"), Filename: "photo.png", ContentType: "image/png", MediaType: "image", Caption: "look at this"})
	if err != nil {
		t.Fatalf("send media: %v", err)
	}
	if userMessage.Modality != "image" || len(userMessage.Assets) != 1 || assistantMessage.Content == "" || len(store.assets) != 1 || len(processor.jobs) != 1 {
		t.Fatalf("unexpected media lifecycle: user=%+v assistant=%+v assets=%+v jobs=%+v", userMessage, assistantMessage, store.assets, processor.jobs)
	}
	if store.advancedStage != OnboardingStageSmallTalk {
		t.Fatalf("expected media reply to advance onboarding, got %q", store.advancedStage)
	}
	if assets.req == nil || assets.req.Metadata["user_id"] != "user-1" || assets.req.Metadata["media_type"] != "image" || string(assets.req.Data) != "data" {
		t.Fatalf("asset upload contract mismatch: %+v", assets.req)
	}
	if model.chatCalls != 1 || model.lastChatRequest == nil || len(model.lastChatRequest.Messages) == 0 {
		t.Fatalf("expected multimodal model call, got %d request=%+v", model.chatCalls, model.lastChatRequest)
	}
	current := model.lastChatRequest.Messages[len(model.lastChatRequest.Messages)-1]
	if len(current.ContentParts) != 1 || current.ContentParts[0].Type != "image_url" || current.ContentParts[0].Url != "https://asset.test/photo.png" {
		t.Fatalf("expected image content part, got %+v", current.ContentParts)
	}

	videoAssets := &fakeAssetStorage{reply: &assetv1.UploadFileReply{FileId: "asset-2", Name: "clip.mp4", ContentType: "video/mp4", Url: "https://asset.test/clip.mp4", Size: 8}}
	usecase.assets = videoAssets
	_, _, err = usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: []byte("video"), Filename: "clip.mp4", ContentType: "video/mp4", MediaType: "video", Caption: "watch this"})
	if err != nil {
		t.Fatalf("send video: %v", err)
	}
	current = model.lastChatRequest.Messages[len(model.lastChatRequest.Messages)-1]
	if len(current.ContentParts) != 1 || current.ContentParts[0].Type != "video_url" || current.ContentParts[0].Url != "https://asset.test/clip.mp4" {
		t.Fatalf("expected video content part, got %+v", current.ContentParts)
	}
}

func TestSendVideoMessageBuildsVideoContentPart(t *testing.T) {
	usecase, _, model, _ := newUsecaseFixture()
	usecase.assets = &fakeAssetStorage{reply: &assetv1.UploadFileReply{FileId: "asset-video", Name: "clip.mp4", ContentType: "video/mp4", Url: "https://asset.test/clip.mp4", Size: 8}}
	_, _, err := usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: []byte("video"), Filename: "clip.mp4", ContentType: "video/mp4", MediaType: "video"})
	if err != nil {
		t.Fatalf("send video: %v", err)
	}
	current := model.lastChatRequest.Messages[len(model.lastChatRequest.Messages)-1]
	if len(current.ContentParts) != 1 || current.ContentParts[0].Type != "video_url" || current.ContentParts[0].Url != "https://asset.test/clip.mp4" {
		t.Fatalf("expected video content part, got %+v", current.ContentParts)
	}
}

func TestSendMediaMessageValidatesMediaAndUploadBoundaries(t *testing.T) {
	for name, request := range map[string]*v1.SendMediaMessageRequest{
		"nil":                     nil,
		"empty data":              {MediaType: "image", ContentType: "image/png"},
		"unknown media":           {Data: []byte("x"), MediaType: "audio", ContentType: "audio/mpeg"},
		"mismatched content type": {Data: []byte("x"), MediaType: "image", ContentType: "video/mp4"},
	} {
		t.Run(name, func(t *testing.T) {
			usecase, _, _, _ := newUsecaseFixture()
			usecase.assets = &fakeAssetStorage{}
			if _, _, err := usecase.SendMediaMessage(companionContext(), request); err == nil {
				t.Fatal("expected media validation error")
			}
		})
	}
	usecase, _, _, _ := newUsecaseFixture()
	usecase.assets = nil
	if _, _, err := usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: []byte("x"), MediaType: "image", ContentType: "image/png"}); err == nil || !strings.Contains(err.Error(), "asset storage") {
		t.Fatalf("expected missing asset storage error, got %v", err)
	}
	usecase, _, _, _ = newUsecaseFixture()
	usecase.assets = &fakeAssetStorage{}
	tooLargeImage := make([]byte, 20*1024*1024+1)
	if _, _, err := usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: tooLargeImage, MediaType: "image", ContentType: "image/png"}); err == nil || !strings.Contains(err.Error(), "media data exceeds") {
		t.Fatalf("expected image size limit error, got %v", err)
	}
	usecase, _, _, _ = newUsecaseFixture()
	usecase.assets = &fakeAssetStorage{reply: &assetv1.UploadFileReply{FileId: "", Url: ""}}
	if _, _, err := usecase.SendMediaMessage(companionContext(), &v1.SendMediaMessageRequest{Data: []byte("x"), MediaType: "image", ContentType: "image/png"}); err == nil || !strings.Contains(err.Error(), "incomplete upload") {
		t.Fatalf("expected incomplete upload error, got %v", err)
	}
}

func TestRollContextIfNeededSummarizesOversizedHistoryAndSwallowsModelFailure(t *testing.T) {
	usecase, store, model, _ := newUsecaseFixture()
	store.messages = []data.MessageModel{{Role: "user", Content: strings.Repeat("x", contextRollCharacterThreshold+1)}}
	conversation := store.conversation
	rolled, history, err := usecase.rollContextIfNeeded(companionContext(), conversation, store.messages)
	if err != nil || history != nil || !store.rolled || rolled.Summary != "I hear you." || model.chatCalls != 1 {
		t.Fatalf("expected summary roll: conversation=%+v history=%v rolled=%v calls=%d err=%v", rolled, history, store.rolled, model.chatCalls, err)
	}
	model.chatErr = errors.New("summary unavailable")
	store.rolled = false
	rolled, history, err = usecase.rollContextIfNeeded(companionContext(), conversation, store.messages)
	if err != nil || rolled != conversation || history == nil || store.rolled {
		t.Fatalf("model summary failure should preserve context: %+v %v rolled=%v err=%v", rolled, history, store.rolled, err)
	}
}

func TestRollContextIfNeededLeavesShortHistoryUntouched(t *testing.T) {
	usecase, store, model, _ := newUsecaseFixture()
	history := []data.MessageModel{{Role: "user", Content: "short"}}
	conversation, result, err := usecase.rollContextIfNeeded(companionContext(), store.conversation, history)
	if err != nil || conversation != store.conversation || len(result) != 1 || result[0].Content != "short" || model.chatCalls != 0 || store.rolled {
		t.Fatalf("short history should not roll: conversation=%+v result=%+v calls=%d rolled=%v err=%v", conversation, result, model.chatCalls, store.rolled, err)
	}
}
