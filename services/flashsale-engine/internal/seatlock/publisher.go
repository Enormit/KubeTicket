package seatlock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/redis/go-redis/v9"
)

// OrderPublisher dispatches checkout order messages to SQS FIFO or Redis Streams.
type OrderPublisher interface {
	PublishOrder(ctx context.Context, msg models.OrderQueueMessage) error
}

// RedisStreamPublisher publishes orders to a local Redis Stream when SQS is not configured.
type RedisStreamPublisher struct {
	rdb redis.UniversalClient
}

// NewRedisStreamPublisher creates a Redis Streams order publisher.
func NewRedisStreamPublisher(rdb redis.UniversalClient) *RedisStreamPublisher {
	return &RedisStreamPublisher{rdb: rdb}
}

func (p *RedisStreamPublisher) PublishOrder(ctx context.Context, msg models.OrderQueueMessage) error {
	attrs := observability.InjectTraceContext(ctx)
	if tp := attrs["traceparent"]; tp != "" {
		msg.TraceParent = tp
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: models.RedisOrderStreamKey(),
		Values: map[string]any{
			"order_id":    msg.OrderID,
			"event_id":    msg.EventID,
			"traceparent": msg.TraceParent,
			"payload":     string(raw),
		},
	}).Err()
}

// SQSFIFOPublisher publishes orders to an Amazon SQS FIFO queue.
type SQSFIFOPublisher struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSFIFOPublisher initializes an AWS SDK v2 SQS FIFO publisher.
func NewSQSFIFOPublisher(ctx context.Context, region, queueURL string) (*SQSFIFOPublisher, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &SQSFIFOPublisher{
		client:   sqs.NewFromConfig(cfg),
		queueURL: queueURL,
	}, nil
}

func (p *SQSFIFOPublisher) PublishOrder(ctx context.Context, msg models.OrderQueueMessage) error {
	attrs := observability.InjectTraceContext(ctx)
	if tp := attrs["traceparent"]; tp != "" {
		msg.TraceParent = tp
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	msgAttrs := map[string]sqstypes.MessageAttributeValue{}
	if msg.TraceParent != "" {
		msgAttrs["traceparent"] = sqstypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(msg.TraceParent),
		}
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(p.queueURL),
		MessageBody:            aws.String(string(raw)),
		MessageGroupId:         aws.String(msg.EventID),
		MessageDeduplicationId: aws.String(msg.OrderID),
		MessageAttributes:      msgAttrs,
	})
	return err
}
