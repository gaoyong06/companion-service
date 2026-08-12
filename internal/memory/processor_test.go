package memory

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"companion-service/internal/client"
	"companion-service/internal/conf"
	"companion-service/internal/data"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/go-kratos/kratos/v2/log"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type processorStore struct {
	saved  []*data.MemoryModel
	notify chan struct{}
	err    error
}

func (s *processorStore) SaveMemory(_ context.Context, model *data.MemoryModel) error {
	if s.err != nil {
		return s.err
	}
	s.saved = append(s.saved, model)
	if s.notify != nil {
		s.notify <- struct{}{}
	}
	return nil
}

type processorModel struct {
	embedding     []float32
	chatErr       error
	embedErr      error
	embedReplyNil bool
}

func (m *processorModel) Chat(context.Context, *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return &modelv1.ChatCompletionReply{Content: `{"memories":[{"kind":"preference","content":"likes tea","confidence":0.9,"importance":4}]}`}, nil
}
func (m *processorModel) ChatStream(context.Context, *modelv1.ChatCompletionRequest) (client.ChatStream, error) {
	return nil, errors.New("not used")
}
func (m *processorModel) Embed(context.Context, []string) (*modelv1.CreateEmbeddingReply, error) {
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	if m.embedReplyNil {
		return nil, nil
	}
	return &modelv1.CreateEmbeddingReply{Data: []*modelv1.EmbeddingItem{{Embedding: m.embedding}}}, nil
}
func (m *processorModel) TranscribeAudio(context.Context, *modelv1.TranscribeAudioRequest) (*modelv1.TranscribeAudioReply, error) {
	return nil, errors.New("not used")
}
func (m *processorModel) SynthesizeSpeech(context.Context, *modelv1.SynthesizeSpeechRequest) (*modelv1.SynthesizeSpeechReply, error) {
	return nil, errors.New("not used")
}

func TestShouldSkipMemory(t *testing.T) {
	for _, content := range []string{"不要记住这件事", "Please do not remember this", "don't save this"} {
		if !ShouldSkipMemory(content) {
			t.Fatalf("expected memory skip for %q", content)
		}
	}
	if ShouldSkipMemory("I prefer jazz music") {
		t.Fatal("did not expect memory skip")
	}
}

func TestShouldSkipMemoryNormalizesCaseAndWhitespace(t *testing.T) {
	for _, content := range []string{"  DON'T REMEMBER this  ", "不要保存这条"} {
		if !ShouldSkipMemory(content) {
			t.Fatalf("expected skip marker for %q", content)
		}
	}
}

func TestProcessorEnqueueHonorsDisabledInvalidBlockedAndFullBoundaries(t *testing.T) {
	job := Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "likes tea"}
	if err := (&Processor{}).Enqueue(job); err != nil {
		t.Fatalf("disabled processor should be a no-op: %v", err)
	}
	p := &Processor{enabled: true, queue: make(chan Job, 1), ctx: context.Background(), log: log.NewHelper(log.NewStdLogger(io.Discard))}
	if err := p.Enqueue(Job{}); err != nil {
		t.Fatalf("invalid job should be ignored: %v", err)
	}
	p.blocked.Store("user-1", struct{}{})
	if err := p.Enqueue(job); err != nil {
		t.Fatalf("blocked user should be ignored: %v", err)
	}
	p.blocked.Delete("user-1")
	if err := p.Enqueue(job); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := p.Enqueue(job); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected queue full, got %v", err)
	}
	if err := (&Processor{enabled: true, queue: make(chan Job, 1), ctx: context.Background()}).Enqueue(Job{UserID: "user-1", Content: "do not remember this"}); err != nil {
		t.Fatalf("skip-memory marker should be a no-op: %v", err)
	}
}

func TestNewRocketMQJobQueueValidatesConfigurationBeforeConnecting(t *testing.T) {
	for _, cfg := range []*conf.Queue{nil, {}, {Enabled: true, Driver: "memory"}, {Enabled: true, Driver: "rocketmq", Topic: "topic", GroupName: "group"}} {
		queue, err := newRocketMQJobQueue(cfg, log.NewStdLogger(io.Discard), func(context.Context, Job) error { return nil })
		if cfg == nil || !cfg.Enabled || cfg.Driver != "rocketmq" {
			if err != nil || queue != nil {
				t.Fatalf("disabled/non-rocket queue should be nil, queue=%v err=%v", queue, err)
			}
			continue
		}
		if err == nil || queue != nil {
			t.Fatalf("incomplete rocketmq config should fail before connect, queue=%v err=%v", queue, err)
		}
	}
}

func TestConsumeMemoryMessagesReturnsSuccessAfterProcessingAllMessages(t *testing.T) {
	var received []Job
	messages := []*primitive.MessageExt{
		{Message: primitive.Message{Body: []byte(`{"user_id":"user-1","source_message_id":"msg-1","content":"likes tea"}`)}},
		{Message: primitive.Message{Body: []byte(`{"user_id":"user-2","source_message_id":"msg-2","content":"likes coffee"}`)}},
	}
	result, err := consumeMemoryMessages(context.Background(), messages, func(_ context.Context, job Job) error {
		received = append(received, job)
		return nil
	}, log.NewHelper(log.NewStdLogger(io.Discard)))
	if result != consumer.ConsumeSuccess || err != nil {
		t.Fatalf("expected successful consumption, result=%v err=%v", result, err)
	}
	if len(received) != 2 || received[0].UserID != "user-1" || received[1].Content != "likes coffee" {
		t.Fatalf("unexpected consumed jobs: %+v", received)
	}
}

func TestConsumeMemoryMessagesRetriesWhenHandlerFails(t *testing.T) {
	wantErr := errors.New("database unavailable")
	result, err := consumeMemoryMessages(context.Background(), []*primitive.MessageExt{
		{Message: primitive.Message{Body: []byte(`{"user_id":"user-1","content":"likes tea"}`)}},
	}, func(context.Context, Job) error { return wantErr }, log.NewHelper(log.NewStdLogger(io.Discard)))
	if result != consumer.ConsumeRetryLater || !errors.Is(err, wantErr) {
		t.Fatalf("expected retry result with handler error, result=%v err=%v", result, err)
	}
}

func TestConsumeMemoryMessagesAcknowledgesMalformedMessages(t *testing.T) {
	processed := 0
	result, err := consumeMemoryMessages(context.Background(), []*primitive.MessageExt{
		{Message: primitive.Message{Body: []byte("not-json")}},
		{Message: primitive.Message{Body: []byte(`{"user_id":"user-1","content":"likes tea"}`)}},
	}, func(context.Context, Job) error {
		processed++
		return nil
	}, log.NewHelper(log.NewStdLogger(io.Discard)))
	if result != consumer.ConsumeSuccess || err != nil || processed != 1 {
		t.Fatalf("malformed message should be acknowledged while valid messages are processed, result=%v err=%v processed=%d", result, err, processed)
	}
}

func TestProcessorProcessExtractsAndPersistsEmbedding(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 2, extractor: NewExtractor(&processorModel{embedding: []float32{0.1, 0.2}}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
	if len(store.saved) != 1 || store.saved[0].Content != "likes tea" || len(store.saved[0].Embedding) != 2 {
		t.Fatalf("unexpected persisted memory: %+v", store.saved)
	}
}

func TestProcessorDropsEmbeddingWhenDimensionDoesNotMatch(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 3, extractor: NewExtractor(&processorModel{embedding: []float32{0.1, 0.2}}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
	if len(store.saved) != 1 || len(store.saved[0].Embedding) != 0 {
		t.Fatalf("expected memory without invalid embedding: %+v", store.saved)
	}
}

func TestProcessorForgetUserBlocksPersistenceAndDisabledProcessorIsSafe(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 2, extractor: NewExtractor(&processorModel{embedding: []float32{0.1, 0.2}}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	processor.ForgetUser("user-1")
	processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
	if len(store.saved) != 0 {
		t.Fatalf("blocked user was persisted: %+v", store.saved)
	}
	p, cleanup, err := NewProcessor(&conf.Memory{Enabled: false}, nil, nil, nil, log.NewStdLogger(io.Discard))
	if err != nil || p == nil {
		t.Fatalf("create disabled processor: %v", err)
	}
	cleanup()
}

func TestProcessorStopsWhenExtractionFails(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 1, extractor: NewExtractor(&processorModel{chatErr: errors.New("chat down")}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	if err := processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"}); err == nil {
		t.Fatal("expected extraction error")
	}
	if len(store.saved) != 0 {
		t.Fatalf("failed memory extraction must not persist candidates: %+v", store.saved)
	}
}

func TestProcessorReturnsPersistenceErrorForRocketMQRetry(t *testing.T) {
	wantErr := errors.New("database unavailable")
	store := &processorStore{err: wantErr}
	processor := &Processor{enabled: true, embeddingDimension: 2, extractor: NewExtractor(&processorModel{embedding: []float32{0.1, 0.2}}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	if err := processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected persistence error for RocketMQ retry, got %v", err)
	}
}

func TestProcessorPersistsCandidateWhenEmbeddingFails(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 2, extractor: NewExtractor(&processorModel{embedErr: errors.New("embedding down")}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
	if len(store.saved) != 1 || store.saved[0].Content != "likes tea" || len(store.saved[0].Embedding) != 0 {
		t.Fatalf("embedding failure must keep the extracted memory without a vector: %+v", store.saved)
	}
}

func TestProcessorPersistsCandidateWhenEmbeddingReplyIsNil(t *testing.T) {
	store := &processorStore{}
	processor := &Processor{enabled: true, embeddingDimension: 2, extractor: NewExtractor(&processorModel{embedReplyNil: true}), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
	processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
	if len(store.saved) != 1 || store.saved[0].Content != "likes tea" || len(store.saved[0].Embedding) != 0 {
		t.Fatalf("nil embedding reply must keep the extracted memory without a vector: %+v", store.saved)
	}
}

func TestNewProcessorRunsLocalQueueAndStopsCleanly(t *testing.T) {
	store := &processorStore{notify: make(chan struct{}, 1)}
	processor, cleanup, err := NewProcessor(&conf.Memory{Enabled: true, QueueSize: 1, EmbeddingDimension: 2}, &conf.Queue{}, store, &processorModel{embedding: []float32{0.1, 0.2}}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatalf("create enabled processor: %v", err)
	}
	if err := processor.Enqueue(Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"}); err != nil {
		t.Fatalf("enqueue local job: %v", err)
	}
	select {
	case <-store.notify:
	case <-time.After(time.Second):
		t.Fatal("local processor did not persist queued memory")
	}
	cleanup()
	if len(store.saved) != 1 {
		t.Fatalf("unexpected persisted memories: %+v", store.saved)
	}
}

func TestNewProcessorRejectsIncompleteEnabledRocketMQConfiguration(t *testing.T) {
	store := &processorStore{}
	processor, cleanup, err := NewProcessor(&conf.Memory{Enabled: true}, &conf.Queue{Enabled: true, Driver: "rocketmq", Topic: "memory", GroupName: "group"}, store, &processorModel{}, log.NewStdLogger(io.Discard))
	if err == nil || processor != nil || cleanup != nil {
		t.Fatalf("expected incomplete RocketMQ configuration error, processor=%v cleanup=%v err=%v", processor, cleanup == nil, err)
	}
}
