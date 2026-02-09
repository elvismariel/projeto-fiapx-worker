package messaging

import (
	"encoding/json"
	"fmt"
	"log"
	"video-processor-worker/internal/core/ports"

	"github.com/nats-io/nats.go"
)

type NatsConsumerAdapter struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	handler func(videoID int64, filename string) error
}

type uploadEvent struct {
	VideoID  int64  `json:"video_id"`
	Filename string `json:"filename"`
}

func NewNatsConsumerAdapter(url string, handler func(videoID int64, filename string) error) (ports.EventConsumer, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("error connecting to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("error getting JetStream context: %w", err)
	}

	return &NatsConsumerAdapter{
		nc:      nc,
		js:      js,
		handler: handler,
	}, nil
}

func (a *NatsConsumerAdapter) Listen() error {
	log.Println("👂 Listening for NATS JetStream events on subject 'upload'...")

	// Durable push-based consumer
	sub, err := a.js.Subscribe("upload", func(m *nats.Msg) {
		var event uploadEvent
		if err := json.Unmarshal(m.Data, &event); err != nil {
			log.Printf("❌ Error unmarshaling event: %v", err)
			return
		}

		log.Printf("📥 Received event: video_id=%d, filename=%s", event.VideoID, event.Filename)

		if err := a.handler(event.VideoID, event.Filename); err != nil {
			log.Printf("❌ Error handling event: %v", err)
			// Nak so it can be retried if needed, but for now we just log
			m.Nak()
			return
		}

		m.Ack()
	}, nats.Durable("worker"), nats.ManualAck())

	if err != nil {
		return fmt.Errorf("error subscribing to NATS: %w", err)
	}

	log.Printf("✅ Subscribed to %s", sub.Subject)
	return nil
}
