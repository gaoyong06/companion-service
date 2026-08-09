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
	"github.com/go-kratos/kratos/v2/log"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type processorStore struct {
	saved  []*data.MemoryModel
	notify chan struct{}
}

func (s *processorStore) SaveMemory(_ context.Context, model *data.MemoryModel) error {
	s.saved = append(s.saved, model)
	if s.notify != nil {
		s.notify <- struct{}{}
	}
	return nil
}

type processorModel struct {
	embedding []float32
	chatErr   error
	embedErr  error
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
		queue, err := newRocketMQJobQueue(cfg, log.NewStdLogger(io.Discard), func(context.Context, Job) {})
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

func TestProcessorStopsWhenExtractionOrEmbeddingFails(t *testing.T) {
	for name, models := range map[string]*processorModel{
		"extraction": {chatErr: errors.New("chat down")},
		"embedding":  {embedding: []float32{0.1}, embedErr: errors.New("embedding down")},
	} {
		t.Run(name, func(t *testing.T) {
			store := &processorStore{}
			processor := &Processor{enabled: true, embeddingDimension: 1, extractor: NewExtractor(models), store: store, log: log.NewHelper(log.NewStdLogger(io.Discard))}
			processor.process(context.Background(), Job{UserID: "user-1", SourceMessageID: "msg-1", Content: "I like tea"})
			if len(store.saved) != 0 {
				t.Fatalf("failed memory pipeline must not persist candidates: %+v", store.saved)
			}
		})
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
