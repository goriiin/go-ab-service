package main

import (
	"log"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

func main() {
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "event_consumer",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	defer consumer.Close()

	topic := "ab_assignment_events"
	if err := consumer.Subscribe(topic, nil); err != nil {
		log.Fatalf("Failed to subscribe to topic %s: %v", topic, err)
	}

	log.Printf("Listening for events on topic: %s", topic)
	for {
		msg, err := consumer.ReadMessage(-1)
		if err != nil {
			log.Printf("Consumer error: %v", err)
			continue
		}
		log.Printf("Received message: %s", string(msg.Value))
	}
}
