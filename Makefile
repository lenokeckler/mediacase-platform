.PHONY: up down logs build test fmt vet

up:
	docker compose up --build -d
	@echo "\n✅ System working:"
	@echo "   Coordinator → http://localhost:8080"
	@echo "   MinIO UI    → http://localhost:9001  (minioadmin / minioadmin)"
	@echo "   Prometheus  → http://localhost:9090"
	@echo "   Grafana     → http://localhost:3001  (admin / admin)"

down:
	docker compose down -v

logs:
	docker compose logs -f

build:
	go build ./...

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

ps:
	docker compose ps

hooks:
hooks:
	git config core.hooksPath .githooks
	@echo Git hooks instalados con exito.