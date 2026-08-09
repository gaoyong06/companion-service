package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"companion-service/internal/conf"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/apache/rocketmq-client-go/v2/rlog"
	"github.com/go-kratos/kratos/v2/log"
)

type jobQueue interface {
	Publish(context.Context, Job) error
	Close() error
}

type rocketMQJobQueue struct {
	producer rocketmq.Producer
	consumer rocketmq.PushConsumer
	topic    string
	log      *log.Helper
}

func newRocketMQJobQueue(c *conf.Queue, logger log.Logger, handler func(context.Context, Job)) (*rocketMQJobQueue, error) {
	if c == nil || !c.Enabled || c.Driver != "rocketmq" {
		return nil, nil
	}
	if len(c.NameServers) == 0 || c.GroupName == "" || c.Topic == "" {
		return nil, fmt.Errorf("rocketmq memory queue configuration is incomplete")
	}
	rlog.SetLogLevel("warn")
	credentials := primitive.Credentials{AccessKey: c.AccessKey, SecretKey: c.SecretKey}
	p, err := rocketmq.NewProducer(
		producer.WithNsResolver(primitive.NewPassthroughResolver(c.NameServers)),
		producer.WithGroupName(c.GroupName+"_producer"),
		producer.WithRetry(2),
		producer.WithCredentials(credentials),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq memory producer: %w", err)
	}
	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start rocketmq memory producer: %w", err)
	}
	consumerClient, err := rocketmq.NewPushConsumer(
		consumer.WithNsResolver(primitive.NewPassthroughResolver(c.NameServers)),
		consumer.WithGroupName(c.GroupName+"_consumer"),
		consumer.WithCredentials(credentials),
	)
	if err != nil {
		_ = p.Shutdown()
		return nil, fmt.Errorf("create rocketmq memory consumer: %w", err)
	}
	queue := &rocketMQJobQueue{producer: p, consumer: consumerClient, topic: c.Topic, log: log.NewHelper(logger)}
	if err := consumerClient.Subscribe(c.Topic, consumer.MessageSelector{}, func(ctx context.Context, messages ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, message := range messages {
			var job Job
			if err := json.Unmarshal(message.Body, &job); err != nil {
				queue.log.Errorf("decode memory job failed: %v", err)
				return consumer.ConsumeSuccess, nil
			}
			handler(ctx, job)
		}
		return consumer.ConsumeSuccess, nil
	}); err != nil {
		_ = p.Shutdown()
		return nil, fmt.Errorf("subscribe memory topic: %w", err)
	}
	if err := consumerClient.Start(); err != nil {
		_ = p.Shutdown()
		return nil, fmt.Errorf("start rocketmq memory consumer: %w", err)
	}
	return queue, nil
}

func (q *rocketMQJobQueue) Publish(ctx context.Context, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode memory job: %w", err)
	}
	result, err := q.producer.SendSync(ctx, primitive.NewMessage(q.topic, body))
	if err != nil {
		return fmt.Errorf("publish memory job: %w", err)
	}
	if result.Status != primitive.SendOK {
		return fmt.Errorf("publish memory job returned status %v", result.Status)
	}
	return nil
}

func (q *rocketMQJobQueue) Close() error {
	var firstErr error
	if q.consumer != nil {
		if err := q.consumer.Shutdown(); err != nil {
			firstErr = err
		}
	}
	if q.producer != nil {
		if err := q.producer.Shutdown(); firstErr == nil && err != nil {
			firstErr = err
		}
	}
	return firstErr
}
