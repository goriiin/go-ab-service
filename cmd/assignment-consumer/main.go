package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/segmentio/kafka-go"
)

func main() {
	kafkaBrokers := []string{"kafka:9092"}
	topic := "ab_assignment_events"
	groupID := "assignment-consumer-group"

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  kafkaBrokers,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
	defer reader.Close()

	log.Printf("INFO: Starting assignment event consumer for topic '%s'...", topic)
	log.Println("INFO: Press Ctrl+C to stop.")

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("INFO: Shutdown signal received. Closing consumer...")
		cancel()
	}()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}

			log.Printf("ERROR: Could not read message: %v", err)

			continue
		}

		log.Printf(
			"Received Assignment Event:\n\tPartition: %d\n\tOffset: %d\n\tKey (UserID): %s\n\tPayload: %s\n",
			msg.Partition,
			msg.Offset,
			string(msg.Key),
			string(msg.Value),
		)
	}

	log.Println("INFO: Assignment consumer stopped.")
}
