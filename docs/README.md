# Boddle Authentication System Documentation

This documentation covers the migration from the current Rails-based authentication system to a new Go-based authentication gateway called "Reservoir".

## Documentation Structure

### Current System
Documentation of the existing Rails LMS authentication implementation.

- **[Authentication Overview](current-system/authentication.md)** - Complete overview of current auth system
- **[Database Schema](current-system/database-schema.md)** - Database tables and relationships
- **[OAuth Providers](current-system/oauth-providers.md)** - Google, Clever, and iCloud SSO implementation
- **[Login Flows](current-system/login-flows.md)** - Detailed authentication flows

### New System
Documentation of the Go authentication gateway architecture.

- **[Architecture Overview](new-system/architecture.md)** - System architecture and design decisions
- **[Go Gateway Implementation](new-system/go-gateway.md)** - Go service implementation details
- **[JWT Design](new-system/jwt-design.md)** - JWT token structure and validation
- **[Security Features](new-system/security.md)** - Rate limiting, token blacklist, security measures

### Operations

Deployment, observability, and the operational runbooks introduced in the post-incident hardening work for [PIR 2026-05-19](pre-release-hardening/PIR_2026_05_19_ACTION_ITEMS.md).

- **[Deployment](DEPLOYMENT.md)** - AWS ECS / CloudFormation deployment, SSM parameters
- **[Observability](OBSERVABILITY.md)** - New Relic APM, Prometheus metrics, structured logging
- **[Capacity Planning](CAPACITY_PLANNING.md)** - Sizing, scaling, connection budgets
- **[CPU Profiling](operations/CPU_PROFILING.md)** - When and how to capture pprof against Reservoir under load (PIR #8)
- **[Rollback](operations/ROLLBACK.md)** - Decision tree, two-executor protocol, recovery signals (PIR #9)
- **[Post-Launch Monitoring](operations/POST_LAUNCH_MONITORING.md)** - Named-owner ritual for every major deploy (PIR #10)
- **[Pre-Deploy Smoke Test](operations/PRE_DEPLOY_SMOKE_TEST.md)** - Write-path verification before flipping LMS traffic (PIR #11)
- **[Incident Response Checklist](operations/INCIDENT_RESPONSE_CHECKLIST.md)** - During-incident playbook, including measuring user impact (PIR #13)
- **[DB Reader Setup](operations/DB_READER_SETUP.md)** - Per-environment SSM configuration for the read replica (PIR #14)

### Pre-release hardening

Tracking docs for the work that preceded the production re-deploy.

- **[PIR 2026-05-19 Action Items](pre-release-hardening/PIR_2026_05_19_ACTION_ITEMS.md)** - Code-side tracking against the ClickUp PIR (item status, branches, follow-ups)
- **[Reservoir Security Review](pre-release-hardening/reservoir-security-review.md)** - 2026-05-22 whole-system security review (5 HIGH findings + IdP alternatives)
- **[User Impact Measurement](pre-release-hardening/USER_IMPACT_MEASUREMENT.md)** - SQL/log templates for measuring incident user impact (auth-specific)

### Migration
Documentation for transitioning from Rails to Go authentication.

- **[Rails Changes Required](migration/rails-changes.md)** - Changes needed in Rails LMS
- **[Rollout Strategy](migration/rollout-strategy.md)** - Phased rollout plan
- **[Testing Plan](migration/testing-plan.md)** - Testing strategy and verification

### Diagrams
System architecture diagrams and flow charts.

- **[System Diagrams](diagrams/system-architecture.md)** - Architecture diagrams
- **[Authentication Flows](diagrams/authentication-flows.md)** - Sequence diagrams for each auth method
- **[Database Diagrams](diagrams/database-schema.md)** - Database entity relationships

## Quick Reference

### Current System
- **Framework**: Ruby on Rails
- **Auth Method**: bcrypt with `has_secure_password`
- **Session Storage**: Cookie-based (6-hour expiry)
- **Database**: PostgreSQL
- **OAuth Providers**: Google OAuth2, Clever SSO, iCloud Sign In
- **Magic Links**: Login tokens (5-minute expiry)

### New System
- **Language**: Go 1.22+
- **Framework**: Gin
- **Token Type**: JWT (HS256)
- **Session Storage**: Stateless (JWT)
- **Database**: Shared PostgreSQL with Rails
- **Cache**: Redis (rate limiting, token blacklist)
- **Token Expiry**: 6 hours (access), 30 days (refresh)

## Architecture at a Glance

```
Clients (Web, Mobile, Game)
    ↓
Go Auth Gateway (Reservoir)
    ├─ Email/Password Login
    ├─ Google OAuth2
    ├─ Clever SSO
    ├─ iCloud Sign In
    └─ Login Tokens
    ↓
JWT Token Issued
    ↓
Rails LMS validates JWT
    ↓
Business Logic
```

## Getting Started

1. **Understand Current System**: Start with [Authentication Overview](current-system/authentication.md)
2. **Learn New Architecture**: Read [Architecture Overview](new-system/architecture.md)
3. **Review Migration Plan**: See [Rails Changes Required](migration/rails-changes.md)
4. **Implementation Timeline**: See main [implementation plan](../README.md)

## Key Benefits of Migration

| Aspect | Current (Rails) | New (Go Gateway) |
|--------|----------------|------------------|
| Authentication | Coupled to Rails | Centralized service |
| Token Type | Cookie sessions | JWT (stateless) |
| Mobile Support | Limited | Excellent |
| Scaling | Session affinity required | Stateless, easy to scale |
| Performance | Good | Excellent (Go) |
| Unified Auth | Separate mechanisms | Single gateway |

## Timeline

- **Phase 1**: Foundation & Email/Password - 2 weeks
- **Phase 2**: Rate Limiting & Security - 1 week
- **Phase 3**: Google OAuth2 - 1 week
- **Phase 4**: Clever SSO - 1 week
- **Phase 5**: iCloud Sign In - 1 week
- **Phase 6**: Login Tokens - 1 week
- **Phase 7**: Rails Integration - 2 weeks

**Total**: 9 weeks to production-ready

## Contact & Questions

For questions about this documentation or the migration project, contact the engineering team.

## Related Resources

- [Main Implementation Plan](../README.md)
- [Go Project Structure](../internal/)
- [Rails LMS Repository](../learning_management_system/)
