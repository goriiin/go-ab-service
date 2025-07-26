.PHONY: help up down logs clean test

help:
	@echo "Available commands:"
	@echo "  make up      - Start all services in detached mode."
	@echo "  make down    - Stop all services."
	@echo "  make logs    - Follow logs from all services."
	@echo "  make clean   - Stop all services and remove data volumes."
	@echo "  make test    - Run end-to-end integration tests."

up:
	@echo "Starting local development environment..."
	@docker compose up -d --build
	@echo "Environment started."

down:
	@echo "Stopping local development environment..."
	@docker compose down
	@echo "Environment stopped."

logs:
	@echo "Following logs..."
	@docker compose logs -f

clean:
	@echo "Stopping services and cleaning up data volumes..."
	@docker compose down -v
	@echo "Cleanup complete."

test:
	@echo "Starting end-to-end test environment..."
	@docker compose -f docker-compose.test.yml up --build --abort-on-container-exit
	@echo "Cleaning up test environment..."
	@docker compose -f docker-compose.test.yml down
	@echo "Test run complete."