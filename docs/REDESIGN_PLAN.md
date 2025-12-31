## Plan: Multi-tenant Backend Redesign with GORM

Refactor the Go backend from a simple Redis-based service to a full multi-tenant architecture using GORM, PostgreSQL, and schema-based tenant isolation. Introduces user authentication with OAuth, new tenant-scoped handlers (profile, account, domains, filesystem), and a modular `RegisterRoutes()` pattern.

### Steps

1. **Add GORM + PostgreSQL dependencies and database initialization** - Create [db/db.go](db/db.go) with connection setup using `DATABASE_URL`, configure gorm-multitenancy with schema-based isolation, and add auto-migration support.

2. **Restructure handlers into sections packages** - Move [handlers/chat.go](handlers/chat.go) to `sections/tenant/chat/` and [handlers/image.go](handlers/image.go) to `sections/tenant/images/`, adding `RegisterRoutes(*gin.Engine, *Dependencies)` to each.

3. **Implement user authentication package** - Create `sections/common/users/` with registration, login, password reset handlers, JWT middleware, and OAuth flows for Google, Facebook, and TikTok using `golang.org/x/oauth2`.

4. **Add new tenant handlers** - Implement `sections/tenant/profile/`, `sections/tenant/account/`, `sections/tenant/domains/`, and `sections/tenant/filesystem/` packages with CRUD handlers and GORM models.

5. **Create multi-tenant models** - Define GORM models in each section's `model.go` with tenant-scoped queries using gorm-multitenancy's `TenantTabler` interface.

6. **Refactor main.go** - Update [main.go](main.go) to initialize database, apply middleware (tenant resolution + auth), and call `RegisterRoutes()` from each section package.

### Further Considerations

1. **Multi-tenancy isolation strategy?** Schema-per-tenant (recommended for data isolation, gorm-multitenancy default) / Row-level with `tenant_id` column (simpler, less isolation) / Hybrid approach

Yes, use schema-per-tenant.

2. **Should Redis remain for chat session caching?** Keep Redis for real-time chat + use Postgres for persistence / Migrate fully to Postgres / Make it configurable

Yes, keep Redis for chat storage, also cache filesystem contents in Redis (for section/tenant/filesystem)

3. **Domain handler external integration?** Mock domain registry for MVP / Integrate with a real registrar API (Namecheap, Cloudflare, etc.) / Just store domain records without registration

Add modular approach with support for Namecheap, Cloudflare, OpenSRS. Allow selecting implementation in config.go

4. **Authentication token strategy?** JWT tokens (stateless, scalable) / Server-side sessions in Redis (more control) / Both with configurable option

Yes, use JWT with es512 private key in JWT_PRIVATE_KEY environment variable.