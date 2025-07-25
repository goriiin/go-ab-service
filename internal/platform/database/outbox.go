package database

import (
	"log"

	"github.com/gocql/gocql"
)

type OutboxMessage struct {
	ID      gocql.UUID
	Payload string
	Topic   string
}

func (cs *CassandraSession) InsertOutboxMessage(message OutboxMessage) error {
	batch := cs.Session.NewBatch(gocql.LoggedBatch)
	batch.Query(
		"INSERT INTO outbox (id, payload, topic) VALUES (?, ?, ?)",
		message.ID, message.Payload, message.Topic,
	)

	if err := cs.Session.ExecuteBatch(batch); err != nil {
		log.Printf("Failed to insert outbox message: %v", err)
		return err
	}

	return nil
}
