.PHONY: help build run test clean docker-build docker-up docker-down build-app build-container cf-publish deploy

# Variables
APP_NAME=reservoir
DOCKER_IMAGE=boddle/reservoir
DOCKER_TAG=latest
CONTAINER_REPO=210662219476.dkr.ecr.us-east-1.amazonaws.com
CONTAINER_NAME=boddle-learning/reservoir
cfpublish := docker run --rm -v $(CURDIR)/.cloudformation:/src -w /src -e AWS_REGION=${CLOUD_OPSREGION} theonestack/cfhighlander cfpublish

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Go binary
	@echo "Building $(APP_NAME)..."
	@go build -o $(APP_NAME) cmd/server/main.go
	@echo "Build complete: ./$(APP_NAME)"

run: ## Run the application locally
	@echo "Running $(APP_NAME)..."
	@go run cmd/server/main.go

test: ## Run tests
	@echo "Running tests..."
	@go test ./... -v

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

deploy: build-app build-container cf-publish ## Full build and publish pipeline

build-app: ## Build the Go binary for Linux (production)
	@echo "Building $(APP_NAME) for Linux..."
	@CGO_ENABLED=0 GOOS=linux go build -o $(APP_NAME) ./cmd/server
	@echo "Build complete: ./$(APP_NAME)"

build-container: ## Build and push Docker image to ECR
	docker build -t $(CONTAINER_REPO)/$(CONTAINER_NAME):$(VERSION) .
	docker push $(CONTAINER_REPO)/$(CONTAINER_NAME):$(VERSION)

cf-publish: ## Publish CloudFormation template
	@echo "Publishing CloudFormation template..."
	$(cfpublish) ${GITOPS_PIPELINE_NAME} -q --version ${VERSION} --dstbucket ${CLOUD_CFTEMPLATES_BUCKET} --dstprefix ${CLOUD_CFTEMPLATES_PREFIX}/${GITOPS_PIPELINE_NAME}
