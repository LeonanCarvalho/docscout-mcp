# Copyright 2026 Leonan Carvalho
# SPDX-License-Identifier: AGPL-3.0-only

BINARY      := docscout-mcp
IMAGE       := ghcr.io/doc-scout/mcp-server
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GOFLAGS     := -ldflags="-s -w -X main.serverVersion=$(VERSION)"
PLATFORMS   := linux/amd64 linux/arm64

.PHONY: help build test lint vet vuln clean \
        run inspector \
        docker-build docker-push docker-run \
        compose-up compose-down compose-logs \
        k8s-deploy k8s-delete helm-install helm-upgrade helm-uninstall \
        release \
        benchmark benchmark-live benchmark-dry

# ── Default target ─────────────────────────────────────────────────────────────
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ── Development ───────────────────────────────────────────────────────────────
build: ## Build the binary for the current platform
	go build $(GOFLAGS) -o $(BINARY) .

build-linux: ## Cross-compile for linux/amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(BINARY)-linux-amd64 .

build-all: ## Cross-compile for all supported platforms
	@for platform in $(PLATFORMS); do \
		OS=$$(echo $$platform | cut -d/ -f1); \
		ARCH=$$(echo $$platform | cut -d/ -f2); \
		OUT=$(BINARY)-$$OS-$$ARCH; \
		echo "Building $$OUT..."; \
		CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH go build $(GOFLAGS) -o bin/$$OUT . ; \
	done

test: ## Run all tests
	go test ./...

test-verbose: ## Run all tests with verbose output
	go test -v ./...

test-race: ## Run tests with race detector
	go test -race ./...

benchmark: build ## Run theoretical benchmark (no API key needed)
	./$(BINARY) --benchmark --version $(VERSION) --output benchmark/RESULTS.md
	@echo "Results written to benchmark/RESULTS.md"

benchmark-live: build ## Run live benchmark (requires ANTHROPIC_API_KEY)
	@if [ -z "$$ANTHROPIC_API_KEY" ]; then echo "Error: ANTHROPIC_API_KEY not set"; exit 1; fi
	./$(BINARY) --benchmark --mode live --version $(VERSION) --output benchmark/RESULTS.md
	@echo "Live results written to benchmark/RESULTS.md"

benchmark-dry: build ## Show benchmark plan without running
	./$(BINARY) --benchmark --dry-run

lint: ## Run golangci-lint
	golangci-lint run

vet: ## Run go vet
	go vet ./...

vuln: ## Run govulncheck for known vulnerabilities
	govulncheck ./...

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY)-linux-amd64
	rm -rf bin/

# ── Local run ─────────────────────────────────────────────────────────────────
run: ## Run the server locally via go run (reads .env.local if present)
	go run .

inspector: ## Launch MCP Inspector against the local server
	npx @modelcontextprotocol/inspector go run .

# ── Docker ────────────────────────────────────────────────────────────────────
docker-build: ## Build Docker image (single-platform)
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-build-multiarch: ## Build and push multi-arch image via buildx
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		--push .

docker-push: ## Push Docker image to registry
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

docker-run: ## Run container in HTTP mode (requires GITHUB_TOKEN and GITHUB_ORG env vars)
	docker run --rm -it \
		-e GITHUB_TOKEN=$(GITHUB_TOKEN) \
		-e GITHUB_ORG=$(GITHUB_ORG) \
		-e HTTP_ADDR=:8080 \
		-p 8080:8080 \
		$(IMAGE):latest

# ── Docker Compose ────────────────────────────────────────────────────────────
compose-up: ## Start HTTP + SQLite stack (detached)
	docker compose up -d --build

compose-up-postgres: ## Start HTTP + PostgreSQL stack (detached)
	docker compose --profile postgres up -d --build

compose-down: ## Stop and remove containers
	docker compose --profile postgres down

compose-logs: ## Tail compose logs
	docker compose logs -f

# ── Kubernetes (raw manifests) ─────────────────────────────────────────────────
K8S_DIR := deploy/k8s

k8s-deploy: ## Apply all K8s manifests (set GITHUB_TOKEN and GITHUB_ORG first)
	@if [ -z "$(GITHUB_TOKEN)" ]; then echo "ERROR: GITHUB_TOKEN is not set"; exit 1; fi
	@if [ -z "$(GITHUB_ORG)" ]; then echo "ERROR: GITHUB_ORG is not set"; exit 1; fi
	kubectl apply -f $(K8S_DIR)/namespace.yaml
	kubectl create secret generic docscout-mcp-secrets \
		--namespace=docscout-mcp \
		--from-literal=GITHUB_TOKEN=$(GITHUB_TOKEN) \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f $(K8S_DIR)/configmap.yaml \
		-f $(K8S_DIR)/pvc.yaml \
		-f $(K8S_DIR)/deployment.yaml \
		-f $(K8S_DIR)/service.yaml

k8s-delete: ## Remove all K8s resources
	kubectl delete namespace docscout-mcp --ignore-not-found

# ── Helm ───────────────────────────────────────────────────────────────────────
HELM_DIR    := deploy/helm
HELM_RELEASE := docscout-mcp
HELM_NS     := docscout-mcp

helm-install: ## Install Helm chart (set GITHUB_TOKEN and GITHUB_ORG)
	helm install $(HELM_RELEASE) $(HELM_DIR) \
		--namespace $(HELM_NS) --create-namespace \
		--set secrets.githubToken=$(GITHUB_TOKEN) \
		--set config.githubOrg=$(GITHUB_ORG)

helm-upgrade: ## Upgrade existing Helm release
	helm upgrade $(HELM_RELEASE) $(HELM_DIR) \
		--namespace $(HELM_NS) \
		--set secrets.githubToken=$(GITHUB_TOKEN) \
		--set config.githubOrg=$(GITHUB_ORG)

helm-uninstall: ## Uninstall Helm release
	helm uninstall $(HELM_RELEASE) --namespace $(HELM_NS)

helm-lint: ## Lint the Helm chart
	helm lint $(HELM_DIR)

helm-template: ## Render Helm templates to stdout
	helm template $(HELM_RELEASE) $(HELM_DIR) \
		--set secrets.githubToken=dummy \
		--set config.githubOrg=my-org

# ── Release ────────────────────────────────────────────────────────────────────
release: test build-all docker-build-multiarch ## Full release: test, build all platforms, push multi-arch image
