# AWS Deployment

Reservoir is deployed to AWS ECS using CloudFormation templates managed by [Cfhighlander](https://github.com/theonestack/cfhighlander).

## CloudFormation Structure

The `.cloudformation/` directory contains two files:

- **`reservoir.cfhighlander.rb`** — Cfhighlander template definition. Declares parameters (CPU, memory, scaling) and wires up the `ecs-service@2.18.0` component with imports from the shared VPC and ECS cluster stacks.
- **`reservoir.config.yaml`** — ECS task and service configuration. Defines the container image, ports, IAM policies, environment variables, and resource limits.

### Default Resource Sizing

| Parameter      | Default |
|----------------|---------|
| CPU            | 512     |
| Memory (MB)    | 1024    |
| DesiredCount   | 1       |
| MinCount       | 1       |
| MaxCount       | 1       |
| EnableScaling  | false   |

## Build and Publish

The Makefile provides targets for the full deployment pipeline:

```bash
make deploy           # Runs build-app, build-container, and cf-publish in sequence
make build-app        # Cross-compiles a static Linux binary (CGO_ENABLED=0 GOOS=linux)
make build-container  # Builds Docker image and pushes to ECR
make cf-publish       # Publishes CloudFormation template via Cfhighlander
```

### ECR Repository

Container images are pushed to:

```
210662219476.dkr.ecr.us-east-1.amazonaws.com/boddle-learning/reservoir
```

## Prerequisites

### ECR Repository

Create the ECR repository before first deploy:

```bash
aws ecr create-repository \
  --repository-name boddle-learning/reservoir \
  --region us-east-1
```

### SSM Parameters

The ECS task receives an `SSM_PATH` environment variable set to `/boddle/${EnvironmentName}/reservoir`. All application configuration should be stored as SSM parameters under this path. Required parameters:

| SSM Key (relative to SSM_PATH) | Description                        |
|---------------------------------|------------------------------------|
| `DB_HOST`                       | PostgreSQL host                    |
| `DB_PORT`                       | PostgreSQL port                    |
| `DB_USER`                       | PostgreSQL username                |
| `DB_PASSWORD`                   | PostgreSQL password                |
| `DB_NAME`                       | PostgreSQL database name           |
| `DB_SSL_MODE`                   | PostgreSQL SSL mode                |
| `REDIS_URL`                     | Redis connection URL               |
| `JWT_SECRET_KEY`                | JWT signing key (min 32 chars)     |
| `JWT_REFRESH_SECRET_KEY`        | Refresh token key (min 32 chars)   |
| `GOOGLE_CLIENT_ID`              | Google OAuth2 client ID            |
| `GOOGLE_CLIENT_SECRET`          | Google OAuth2 client secret        |
| `GOOGLE_REDIRECT_URL`           | Google OAuth2 callback URL         |
| `CLEVER_CLIENT_ID`              | Clever SSO client ID               |
| `CLEVER_CLIENT_SECRET`          | Clever SSO client secret           |
| `CLEVER_REDIRECT_URL`           | Clever SSO callback URL            |
| `ICLOUD_SERVICE_ID`             | Apple Sign In service ID           |
| `ICLOUD_TEAM_ID`                | Apple Sign In team ID              |
| `ICLOUD_KEY_ID`                 | Apple Sign In key ID               |
| `ICLOUD_PRIVATE_KEY_PATH`       | Path to Apple private key file     |
| `ICLOUD_REDIRECT_URL`           | Apple Sign In callback URL         |
| `CORS_ALLOWED_ORIGINS`          | Comma-separated allowed origins    |

### CI/CD Environment Variables

The build pipeline expects these variables to be set by the CI environment:

| Variable                    | Description                                      |
|-----------------------------|--------------------------------------------------|
| `VERSION`                   | Build version / Docker image tag                 |
| `CLOUD_OPSREGION`           | AWS region for CloudFormation operations          |
| `CLOUD_CFTEMPLATES_BUCKET`  | S3 bucket for CloudFormation template storage     |
| `CLOUD_CFTEMPLATES_PREFIX`  | S3 key prefix for templates                      |
| `GITOPS_PIPELINE_NAME`      | Pipeline name used by Cfhighlander for publishing |

## Environment Variables Injected by CloudFormation

The ECS task definition automatically receives these from the CloudFormation stack:

| Variable     | Value                                              |
|--------------|----------------------------------------------------|
| `ENV_NAME`   | `${EnvironmentName}` (e.g., `production`, `staging`) |
| `AWS_REGION` | Current AWS region                                 |
| `ENV_REGION` | Current AWS region                                 |
| `SSM_PATH`   | `/boddle/${EnvironmentName}/reservoir`             |

## IAM Permissions

The ECS task role is granted:

- `ec2:DescribeRegions`
- `ssm:GetParameter`
- `ssm:GetParametersByPath`

## SSM Integration

The application currently loads configuration from environment variables via `envconfig`. To use SSM parameters in production, the app needs a startup step that reads parameters from `SSM_PATH` and sets them as environment variables before `config.Load()` runs. This can be done either:

1. **In the application** — add SSM resolution to `cmd/server/main.go` before config loading.
2. **Via a sidecar/init container** — use a tool like [chamber](https://github.com/segmentio/chamber) or [aws-env](https://github.com/Droplr/aws-env) to inject SSM values as env vars.
