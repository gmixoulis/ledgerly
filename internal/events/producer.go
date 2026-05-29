package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type event struct {
	Type      string    `json:"type"`
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			MaxAttempts:  3,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, eventType string, payload any) error {
	data, err := json.Marshal(event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	if err := p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(eventType),
		Value: data,
	}); err != nil {
		log.Printf("warn: failed to publish event %q: %v", eventType, err)
		return err
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

// NopProducer is used in tests.
type NopProducer struct{}

func (NopProducer) Publish(_ context.Context, _ string, _ any) error { return nil }
