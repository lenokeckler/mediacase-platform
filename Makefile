.PHONY: up down logs build build-workers test fmt vet

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

# Binarios estáticos del worker para repartir a otros nodos (equivale a scripts/build-workers.ps1)
build-workers:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags '-s -w' -o bin/worker-linux-amd64       ./cmd/worker
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags '-s -w' -o bin/worker-windows-amd64.exe ./cmd/worker

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