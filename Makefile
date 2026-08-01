.PHONY: compose-up compose-down compose-build test sqlc migrate-up

compose-up:
	docker compose up --build

compose-down:
	docker compose down

compose-build:
	docker compose build

test:
	cd apps/api && go test ./...
	pnpm test

sqlc:
	cd apps/api && sqlc generate

migrate-up:
	docker compose run --rm migrate
