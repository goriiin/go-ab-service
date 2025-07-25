# File: Makefile

.PHONY: help up down logs clean

help:
	@echo "Available commands:"
	@echo "  make up      - Start all services in detached mode."
	@echo "  make down    - Stop all services."
	@echo "  make logs    - Follow logs from all services."
	@echo "  make clean   - Stop all services and remove data volumes."


up:
	@echo "Starting local development environment..."
	@mkdir -p ./docker/cassandra
	@echo "CREATE KEYSPACE IF NOT EXISTS ab_platform WITH REPLICATION = { 'class': 'SimpleStrategy', 'replication_factor': 1 };" > ./docker/cassandra/init.cql
	@docker-compose up -d --build
	@echo "Environment started. Access MinIO console at http://localhost:9001"

down:
	@echo "Stopping local development environment..."
	@docker-compose down
	@echo "Environment stopped."

logs:
	@echo "Following logs..."
	@docker-compose logs -f

clean:
	@echo "Stopping services and cleaning up data volumes..."
	@docker-compose down -v
	@echo "Cleanup complete."