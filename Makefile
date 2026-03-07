.PHONY: dev-backend dev-frontend build up down test lint format check

dev-backend:
	cd backend && go run .

dev-frontend:
	cd frontend && npm run dev

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

test:
	cd backend && go test ./...

test-integration:
	cd backend && go test -tags=integration ./...

lint:
	cd frontend && npm run lint

format:
	cd frontend && npm run format

check:
	cd frontend && npm run check
