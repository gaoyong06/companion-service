package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	companionclient "companion-service/internal/client"
	"companion-service/internal/conf"
	"companion-service/internal/data"
	"companion-service/internal/lexicon"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

var ErrQueueFull = errors.New("memory processing queue is full")

type Job struct {
	UserID          string `json:"user_id"`
	SourceMessageID string `json:"source_message_id"`
	Content         string `json:"content"`
}

type Processor struct {
	enabled            bool
	embeddingDimension int
	queue              chan Job
	remoteQueue        jobQueue
	extractor          *Extractor
	store              interface {
		SaveMemory(context.Context, *data.MemoryModel) error
	}
	log       *log.Helper
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	blocked   sync.Map
	lifecycle sync.RWMutex
}

type MemoryStore interface {
	SaveMemory(context.Context, *data.MemoryModel) error
}

func NewProcessor(c *conf.Memory, queueConfig *conf.Queue, store MemoryStore, models companionclient.ModelGateway, logger log.Logger) (*Processor, func(), error) {
	queueSize := 100
	embeddingDimension := 1536
	enabled := false
	if c != nil {
		enabled = c.Enabled
		if c.QueueSize > 0 {
			queueSize = int(c.QueueSize)
		}
		if c.EmbeddingDimension > 0 {
			embeddingDimension = int(c.EmbeddingDimension)
		}
	}
	processorContext, cancel := context.WithCancel(context.Background())
	processor := &Processor{
		enabled:            enabled,
		embeddingDimension: embeddingDimension,
		queue:              make(chan Job, queueSize),
		extractor:          NewExtractor(models),
		store:              store,
		log:                log.NewHelper(logger),
		ctx:                processorContext,
		cancel:             cancel,
	}
	var remoteQueue *rocketMQJobQueue
	var err error
	if enabled {
		remoteQueue, err = newRocketMQJobQueue(queueConfig, logger, processor.process)
		if err != nil {
			cancel()
			return nil, nil, err
		}
	}
	if remoteQueue != nil {
		processor.remoteQueue = remoteQueue
	}
	if enabled && remoteQueue == nil {
		processor.wg.Add(1)
		go processor.run()
	}
	return processor, func() {
		processor.cancel()
		processor.wg.Wait()
		if processor.remoteQueue != nil {
			_ = processor.remoteQueue.Close()
		}
	}, nil
}

func (p *Processor) Enqueue(job Job) error {
	if p == nil || !p.enabled || strings.TrimSpace(job.UserID) == "" || strings.TrimSpace(job.Content) == "" {
		return nil
	}
	if ShouldSkipMemory(job.Content) {
		return nil
	}
	if p.isBlocked(job.UserID) {
		return nil
	}
	if p.remoteQueue != nil {
		return p.remoteQueue.Publish(p.ctx, job)
	}
	select {
	case p.queue <- job:
		return nil
	default:
		p.log.Warn("memory processing queue is full")
		return ErrQueueFull
	}
}

func (p *Processor) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.queue:
			if err := p.process(p.ctx, job); err != nil {
				p.log.Warnf("memory processing failed: %v", err)
			}
		}
	}
}

func (p *Processor) process(ctx context.Context, job Job) error {
	if p.isBlocked(job.UserID) {
		return nil
	}
	candidates, err := p.extractor.Extract(ctx, job.Content)
	if err != nil {
		return err
	}
	var embedding []float32
	embeddings, embeddingErr := p.extractor.models.Embed(ctx, []string{job.Content})
	switch {
	case embeddingErr != nil:
		p.log.Warnf("memory embedding unavailable; persisting without vector: %v", embeddingErr)
	case embeddings == nil:
		p.log.Warn("memory embedding response is empty; persisting without vector")
	case len(embeddings.Data) == 0 || embeddings.Data[0] == nil:
		p.log.Warn("memory embedding data is empty; persisting without vector")
	case len(embeddings.Data[0].Embedding) != p.embeddingDimension:
		p.log.Warnf("memory embedding dimension mismatch; persisting without vector: expected=%d actual=%d", p.embeddingDimension, len(embeddings.Data[0].Embedding))
	default:
		embedding = embeddings.Data[0].Embedding
	}
	now := time.Now().UTC()
	for _, candidate := range candidates {
		model := &data.MemoryModel{
			MemoryID:        "mem_" + uuid.NewString(),
			UserID:          job.UserID,
			Layer:           "L1",
			Kind:            candidate.Kind,
			Content:         candidate.Content,
			SourceMessageID: job.SourceMessageID,
			Confidence:      candidate.Confidence,
			Importance:      candidate.Importance,
			Status:          "active",
			CreatedAt:       now,
			UpdatedAt:       now,
			Embedding:       embedding,
		}
		p.lifecycle.RLock()
		blocked := p.isBlocked(job.UserID)
		var saveErr error
		if !blocked && p.store != nil {
			saveErr = p.store.SaveMemory(ctx, model)
		} else if !blocked {
			p.log.Warn("memory store is unavailable")
		}
		p.lifecycle.RUnlock()
		if blocked {
			return nil
		}
		if saveErr != nil {
			return fmt.Errorf("save memory: %w", saveErr)
		}
	}
	return nil
}

func (p *Processor) ForgetUser(userID string) {
	if p == nil || strings.TrimSpace(userID) == "" {
		return
	}
	p.lifecycle.Lock()
	p.blocked.Store(userID, struct{}{})
	p.lifecycle.Unlock()
}

func (p *Processor) isBlocked(userID string) bool {
	_, ok := p.blocked.Load(userID)
	return ok
}

func ShouldSkipMemory(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	for _, marker := range lexicon.ForLocale(string(lexicon.DefaultLocale)).Memory.SkipMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
