package database

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type CassandraSession struct {
	Session *gocql.Session
}

func NewCassandraSession(host, keyspace string) (*gocql.Session, error) {
	cluster := gocql.NewCluster(host)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum // Уровень консистентности по умолчанию для надежности
	cluster.Timeout = 10 * time.Second
	cluster.ProtoVersion = 4 // Рекомендуется для Cassandra 4.x

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create cassandra session: %w", err)
	}

	return session, nil
}

func (cs *CassandraSession) Close() {
	cs.Session.Close()
}
