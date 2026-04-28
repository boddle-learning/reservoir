.PHONY: help build run test test-cover clean docker-build docker-up docker-down docker-logs docker-rebuild deps fmt lint build-app build-container cf-publish build-local

# Variables
APP_NAME=reservoir
DOCKER_IMAGE=boddle/reservoir
DOCKER_TAG=latest
CONTAINER_REPO=210662219476.dkr.ecr.us-east-1.amazonaws.com
CONTAINER_NAME=boddle-learning/reservoir
define run-go
docker run --rm -v $(CURDIR):/src -w /src golang:1.22-alpine sh -c "apk add --no-cache git && $(1)"
endef
cfpublish := docker run --rm --platform=linux/x86_64 -v $(CURDIR)/.cloudformation:/src -w /src -e AWS_REGION=${CLOUD_OPSREGION} theonestack/cfhighlander cfpublish

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: build-app build-container cf-publish ## Full CI build and publish pipeline

build-local: ## Build the Go binary locally (in Docker)
	$(call run-go,go build -o $(APP_NAME) cmd/server/main.go)

run: ## Run the application locally
	@echo "Running $(APP_NAME)..."
	@go run cmd/server/main.go

test: ## Run tests
	$(call run-go,go test ./... -v)

test-cover: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test ./... -cover -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(APP_NAME)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-up: ## Start Docker Compose services
	@echo "Starting Docker Compose services..."
	@docker-compose up -d
	@echo "Services started"

docker-down: ## Stop Docker Compose services
	@echo "Stopping Docker Compose services..."
	@docker-compose down
	@echo "Services stopped"

docker-logs: ## View Docker Compose logs
	@docker-compose logs -f

docker-rebuild: docker-down docker-build docker-up ## Rebuild and restart Docker services

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@echo "Dependencies downloaded"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Code formatted"

lint: ## Run linter
	@echo "Running linter..."
	@golangci-lint run
	@echo "Linting complete"


# Fail-fast guard for required variables: usage guard-VARNAME
guard-%:
	@if [ -z '${${*}}' ]; then echo "ERROR: variable $* is required" >&2; exit 1; fi

build-app: ## Build the Go binary for Linux (production)
	$(call run-go,CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o $(APP_NAME) ./cmd/server)

build-container: guard-VERSION ## Build and push Docker image to ECR
	docker build -t $(CONTAINER_REPO)/$(CONTAINER_NAME):$(VERSION) .
	docker push $(CONTAINER_REPO)/$(CONTAINER_NAME):$(VERSION)

cf-publish: guard-VERSION guard-GITOPS_PIPELINE_NAME guard-CLOUD_CFTEMPLATES_BUCKET guard-CLOUD_CFTEMPLATES_PREFIX ## Publish CloudFormation template
	@echo "Publishing CloudFormation template..."
	$(cfpublish) ${GITOPS_PIPELINE_NAME} -q --version ${VERSION} --dstbucket ${CLOUD_CFTEMPLATES_BUCKET} --dstprefix ${CLOUD_CFTEMPLATES_PREFIX}/${GITOPS_PIPELINE_NAME}
