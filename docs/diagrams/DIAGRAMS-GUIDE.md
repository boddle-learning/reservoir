# 📊 Additional Diagrams Created

I've created comprehensive architecture diagrams in Mermaid format that can be rendered as images.

## What's Been Created

### 4 New Diagram Files

1. **`system-architecture.mmd`** (15 diagrams)
   - Complete system architecture (Kubernetes cluster)
   - Network flow sequence
   - Data flow architecture

2. **`deployment-architecture.mmd`** (3 major diagrams)
   - Kubernetes deployment with 3 namespaces
   - AWS deployment (VPC, ECS Fargate, RDS, ElastiCache)
   - Docker Compose for local development

3. **`database-architecture.mmd`** (5 diagrams)
   - Complete ERD with all tables and relationships
   - Database sharding strategy (future scaling)
   - Connection pooling architecture
   - Backup and disaster recovery
   - Data retention and archival

4. **`monitoring-observability.mmd`** (6 diagrams)
   - Complete monitoring stack (Prometheus, Grafana, ELK)
   - Grafana dashboard layout (5 rows of panels)
   - Alert rules and SLOs
   - Distributed tracing (Jaeger)
   - Log aggregation flow
   - Health check architecture

**Total: 29 detailed diagrams** across 4 files

### Supporting Files

- **`scripts/generate-diagrams.sh`** - Automated image generation script
- **`docs/diagrams/README.md`** - Complete guide on viewing and using diagrams

## How to Use These Diagrams

### Quick View (No Installation)

1. **GitHub/GitLab**: Just view the `.md` files on GitHub - Mermaid renders automatically
2. **Mermaid Live**: Copy content to https://mermaid.live/ for instant preview
3. **VS Code**: Install Mermaid Preview extension

### Generate Images

```bash
# 1. Install mermaid-cli
npm install -g @mermaid-js/mermaid-cli

# 2. Generate all diagrams as PNG and SVG
./scripts/generate-diagrams.sh

# 3. View the preview HTML
open docs/diagrams/images/index.html
```

The script will:
- ✅ Convert all `.mmd` files to PNG (1920x1080)
- ✅ Convert all `.mmd` files to SVG (vector)
- ✅ Create an HTML gallery to preview all diagrams
- ✅ Enable click-to-zoom and downloads

### Output Structure

```
docs/diagrams/images/
├── system-architecture.png       (Full system architecture)
├── system-architecture.svg
├── deployment-architecture.png   (Kubernetes + AWS)
├── deployment-architecture.svg
├── database-architecture.png     (ERD + sharding)
├── database-architecture.svg
├── monitoring-observability.png  (Prometheus + Grafana)
├── monitoring-observability.svg
└── index.html                    (Preview gallery)
```

## Diagram Highlights

### 1. System Architecture (`system-architecture.mmd`)

**Contains:**
- **Complete System**: Load balancer → Auth Gateway (3 replicas) → Rails (5 replicas) → Database cluster
- **External Services**: Google OAuth, Clever SSO, Apple Sign In
- **Monitoring**: Prometheus, Grafana, Sentry, ELK stack
- **Network Flow**: Step-by-step sequence showing request flow
- **Data Flow**: Client → Gateway → Cache → Database → Application

**Key Features:**
- Color-coded components (Auth Gateway = green, Rails = blue, Database = yellow)
- Shows replication (Primary → Replica 1, Replica 2)
- Redis cluster for rate limiting and token blacklist
- Clear separation of concerns

### 2. Deployment Architecture (`deployment-architecture.mmd`)

**Contains:**
- **Kubernetes Deployment**:
  - 4 Namespaces: auth-gateway, rails-lms, data, monitoring
  - Auth Gateway: 3 pods with HPA (3-10)
  - Rails: 5 pods with HPA (5-20)
  - StatefulSets for PostgreSQL and Redis
  - Ingress controller with SSL termination

- **AWS Architecture**:
  - VPC with public/private subnets across 2 AZs
  - ECS Fargate for containers
  - RDS PostgreSQL Multi-AZ with read replica
  - ElastiCache Redis cluster
  - CloudFront CDN
  - Application Load Balancer
  - Secrets Manager for credentials

- **Docker Compose**:
  - 4 services: Go Gateway, Rails, PostgreSQL, Redis
  - 2 dev tools: Adminer (DB UI), Redis Commander
  - Volume management
  - Network configuration

### 3. Database Architecture (`database-architecture.mmd`)

**Contains:**
- **Complete ERD**:
  - All 12 tables with full schemas
  - Primary keys, foreign keys, unique constraints
  - Polymorphic relationships (users → teachers/students/parents)
  - Many-to-many join tables
  - New `refresh_tokens` table for JWT refresh

- **Sharding Strategy (Future)**:
  - Shard 0: Authentication data (centralized)
  - Shards 1-3: Content data by school (distributed)
  - ProxySQL for query routing
  - Read/write split

- **Connection Pooling**:
  - Go Gateway: sqlx pools (50 max conns each)
  - Rails: ActiveRecord pools (25 max conns each)
  - PgBouncer: 1000 client → 200 server connections
  - PostgreSQL: 200 max connections

- **Backup & DR**:
  - Continuous WAL archiving (every 5 min)
  - Daily snapshots (30 day retention)
  - Weekly backups (90 day retention)
  - Monthly archives (1 year retention)
  - DR site in different region (<30s lag)
  - Automated restore testing

- **Data Retention**:
  - Hot data: Last 3 months (Primary DB)
  - Warm data: 3-12 months (Read Replicas)
  - Cold data: >12 months (S3 Archive)
  - Automated archival jobs

### 4. Monitoring & Observability (`monitoring-observability.mmd`)

**Contains:**
- **Complete Monitoring Stack**:
  - Prometheus for metrics collection
  - Grafana for visualization
  - Fluent Bit → Logstash → Elasticsearch → Kibana
  - Jaeger for distributed tracing
  - Sentry for error tracking
  - Node, PostgreSQL, Redis exporters

- **Grafana Dashboard Layout**:
  - Row 1: Overview (Total logins, Success rate, Active users, P99 latency)
  - Row 2: Login Methods (Email, Google, Clever, Token - time series)
  - Row 3: Security (Rate limits, Failed logins, Blocked IPs, JWT validations)
  - Row 4: Performance (Request duration, Throughput, Error rate, Saturation)
  - Row 5: Dependencies (DB latency, DB connections, Redis latency, Hit rate)

- **Alert Rules & SLOs**:
  - Availability SLO: 99.9% (43 min/month error budget)
  - Latency SLO: P95 < 500ms, P99 < 1s
  - Critical alerts: Error > 5%, Service down, DB down (page immediately)
  - Warning alerts: Error > 2%, CPU > 80%, Memory > 85%
  - PagerDuty + Slack + Email routing

- **Distributed Tracing**:
  - Trace ID propagation across services
  - Span hierarchy: auth.login → redis.ratelimit → db.query → bcrypt.compare → jwt.generate
  - Shows timing for each operation
  - Full trace: Go Gateway (28ms) + Rails API (9ms) = 37ms total

- **Log Aggregation**:
  - Sources: Go (JSON), Rails (text), Nginx (access logs), System (syslog)
  - Fluent Bit collection with local buffering
  - Logstash for parsing and enrichment
  - Elasticsearch with 7-day retention
  - Kibana for search and visualization
  - ElastAlert for log-based alerts

- **Health Checks**:
  - Load balancer checks every 10s
  - Dependencies: PostgreSQL (SELECT 1), Redis (PING), Disk (>10% free), Memory (<90%)
  - Response types: Healthy (200), Degraded (200 with warnings), Unhealthy (503)
  - Automatic removal from load balancer pool

## Diagram Features

### Professional Styling
- ✅ Color-coded components by type
- ✅ Clear separation of layers (client, gateway, data, external)
- ✅ Arrows showing data flow direction
- ✅ Notes explaining complex flows
- ✅ Legends and labels

### Production-Ready
- ✅ Actual component names (Prometheus, Grafana, ELK)
- ✅ Real port numbers (8080, 3000, 5432, 6379)
- ✅ Specific resource limits (256Mi RAM, 500m CPU)
- ✅ Replica counts (3 Gateway, 5 Rails)
- ✅ Timeout values (10s health check, 5s timeout)

### Comprehensive Coverage
- ✅ All deployment targets (Kubernetes, AWS, Docker Compose)
- ✅ Complete database design (12 tables, all relationships)
- ✅ Full monitoring stack (metrics, logs, traces, errors)
- ✅ Security features (rate limiting, token blacklist)
- ✅ High availability (replication, backups, DR)

## Use Cases

### 1. Presentations
Export as PNG/SVG for slides:
```bash
./scripts/generate-diagrams.sh
# Use PNGs in PowerPoint, Keynote, Google Slides
```

### 2. Documentation
Embed in internal wikis:
- **Confluence**: Install Mermaid plugin, paste code
- **Notion**: Export as PNG and upload
- **GitBook**: Mermaid renders automatically

### 3. Code Reviews
Include in PR descriptions:
```markdown
## Architecture Changes

See updated system architecture:
![System Architecture](docs/diagrams/images/system-architecture.png)
```

### 4. Onboarding
Give new team members visual overview:
- Print deployment architecture
- Walk through database ERD
- Show monitoring dashboard layouts

### 5. Planning Meetings
Display during architecture discussions:
- Project to screen
- Discuss trade-offs
- Annotate with notes

## Next Steps

### Generate Images Now

```bash
# Install mermaid-cli (one time)
npm install -g @mermaid-js/mermaid-cli

# Generate all diagrams
./scripts/generate-diagrams.sh

# Open preview
open docs/diagrams/images/index.html
```

### Customize Diagrams

Edit `.mmd` files to:
- Add your specific IP addresses
- Update resource limits
- Add/remove components
- Change colors or styling

Then regenerate:
```bash
./scripts/generate-diagrams.sh
```

### Share with Team

**Option 1: Share Images**
```bash
# Upload to internal wiki/confluence
cp docs/diagrams/images/*.png /path/to/wiki/
```

**Option 2: Share HTML Preview**
```bash
# Host on internal web server
cp -r docs/diagrams/images/* /var/www/html/diagrams/
```

**Option 3: Email**
```bash
# Zip images
cd docs/diagrams/images
zip -r ../../diagrams.zip *.png *.svg index.html
# Email diagrams.zip
```

## Summary

You now have:

- ✅ **29 detailed architectural diagrams**
- ✅ **4 diagram categories** (System, Deployment, Database, Monitoring)
- ✅ **Automated image generation script**
- ✅ **Interactive HTML preview gallery**
- ✅ **Complete usage documentation**

The diagrams cover:
- Complete system architecture (current & future)
- Kubernetes and AWS deployments
- Full database design with ERD
- Comprehensive monitoring stack
- All authentication flows
- Security features
- High availability setup

**Ready to generate images?**
```bash
npm install -g @mermaid-js/mermaid-cli
./scripts/generate-diagrams.sh
open docs/diagrams/images/index.html
```

Need help viewing or customizing? See `docs/diagrams/README.md` for detailed instructions.
