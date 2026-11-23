package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrPermanent indicates a non-retryable message error (e.g., malformed payload or seat conflict).
var ErrPermanent = errors.New("permanent message error")

// MessageHandler processes one order queue message.
type MessageHandler func(ctx context.Context, msg models.OrderQueueMessage) error

// Consumer runs a blocking queue consumption loop until ctx is canceled.
type Consumer interface {
	Run(ctx context.Context, handler MessageHandler) error
}

// RedisStreamConsumer consumes order messages from Redis Streams using Consumer Groups.
type RedisStreamConsumer struct {
	rdb          redis.UniversalClient
	stream       string
	group        string
	consumerName string
	logger       *slog.Logger
	metrics      *observability.Metrics
}

// NewRedisStreamConsumer initializes a Redis Stream consumer.
func NewRedisStreamConsumer(rdb redis.UniversalClient, logger *slog.Logger, metrics *observability.Metrics) *RedisStreamConsumer {
	return &RedisStreamConsumer{
		rdb:          rdb,
		stream:       models.RedisOrderStreamKey(),
		group:        "order-workers",
		consumerName: "worker-" + uuid.NewString()[:8],
		logger:       logger,
		metrics:      metrics,
	}
}

func (c *RedisStreamConsumer) Run(ctx context.Context, handler MessageHandler) error {
	if err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumerName,
			Streams:  []string{c.stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		for _, stream := range res {
			for _, xmsg := range stream.Messages {
				c.processRedisMessage(ctx, xmsg, handler)
			}
		}
	}
}

func (c *RedisStreamConsumer) processRedisMessage(ctx context.Context, xmsg redis.XMessage, handler MessageHandler) {
	rawPayload, _ := xmsg.Values["payload"].(string)
	var msg models.OrderQueueMessage
	if err := json.Unmarshal([]byte(rawPayload), &msg); err != nil {
		if c.logger != nil {
			c.logger.Error("invalid order stream payload", slog.String("msg_id", xmsg.ID), slog.Any("error", err))
		}
		_ = c.rdb.XAck(ctx, c.stream, c.group, xmsg.ID).Err()
		recordQueueMetric(c.metrics, "redis-streams", "invalid")
		return
	}

	msgCtx := ctx
	if msg.TraceParent != "" {
		msgCtx = observability.ExtractTraceContext(ctx, map[string]string{"traceparent": msg.TraceParent})
	}

	err := handler(msgCtx, msg)
	ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer ackCancel()

	if err == nil {
		_ = c.rdb.XAck(ackCtx, c.stream, c.group, xmsg.ID).Err()
		recordQueueMetric(c.metrics, "redis-streams", "ok")
		return
	}

	if errors.Is(err, ErrPermanent) {
		if c.logger != nil {
			observability.WithContext(msgCtx, c.logger).Warn("acknowledging permanent failure", slog.String("order_id", msg.OrderID), slog.Any("error", err))
		}
		_ = c.rdb.XAck(ackCtx, c.stream, c.group, xmsg.ID).Err()
		recordQueueMetric(c.metrics, "redis-streams", "permanent_error")
		return
	}

	if c.logger != nil {
		observability.WithContext(msgCtx, c.logger).Error("transient order error, leaving unacked", slog.String("order_id", msg.OrderID), slog.Any("error", err))
	}
	recordQueueMetric(c.metrics, "redis-streams", "retry")
}

// SQSFIFOConsumer consumes order messages from Amazon SQS FIFO.
type SQSFIFOConsumer struct {
	client   *sqs.Client
	queueURL string
	logger   *slog.Logger
	metrics  *observability.Metrics
}

// NewSQSFIFOConsumer creates an SQS FIFO consumer using AWS SDK v2 default config.
func NewSQSFIFOConsumer(ctx context.Context, region, queueURL string, logger *slog.Logger, metrics *observability.Metrics) (*SQSFIFOConsumer, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &SQSFIFOConsumer{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
		logger:   logger,
		metrics:  metrics,
	}, nil
}

func (c *SQSFIFOConsumer) Run(ctx context.Context, handler MessageHandler) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(c.queueURL),
			MaxNumberOfMessages:   10,
			WaitTimeSeconds:       20,
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(time.Second)
			continue
		}

		for _, m := range out.Messages {
			var msg models.OrderQueueMessage
			if err := json.Unmarshal([]byte(aws.ToString(m.Body)), &msg); err != nil {
				_, _ = c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(c.queueURL),
					ReceiptHandle: m.ReceiptHandle,
				})
				recordQueueMetric(c.metrics, "sqs-fifo", "invalid")
				continue
			}

			msgCtx := ctx
			if attr, ok := m.MessageAttributes["traceparent"]; ok && attr.StringValue != nil {
				msgCtx = observability.ExtractTraceContext(ctx, map[string]string{"traceparent": *attr.StringValue})
			} else if msg.TraceParent != "" {
				msgCtx = observability.ExtractTraceContext(ctx, map[string]string{"traceparent": msg.TraceParent})
			}

			err := handler(msgCtx, msg)
			if err == nil || errors.Is(err, ErrPermanent) {
				ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				_, _ = c.client.DeleteMessage(ackCtx, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(c.queueURL),
					ReceiptHandle: m.ReceiptHandle,
				})
				ackCancel()
				status := "ok"
				if err != nil {
					status = "permanent_error"
				}
				recordQueueMetric(c.metrics, "sqs-fifo", status)
			} else {
				recordQueueMetric(c.metrics, "sqs-fifo", "retry")
			}
		}
	}
}

func recordQueueMetric(metrics *observability.Metrics, queue, status string) {
	if metrics != nil {
		metrics.QueueProcessedTotal.WithLabelValues(queue, status).Inc()
	}
}
