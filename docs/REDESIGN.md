
Redesign and refactor this Go backend.

1. Use gorm and gorm-multitenancy (https://pkg.go.dev/github.com/bartventer/gorm-multitenancy/v8) with postgres backend. Use DATABASE_URL environment variable.

2. Use gorm-multitenancy gin integration (https://pkg.go.dev/github.com/bartventer/gorm-multitenancy/middleware/gin/v8)

3. Move chat and images handlers to a new package for each under sections/tenant/{handler} add RegisterRoutes(*gin.Engine) public func to each package. Call these from main.go

4. Add new users (sections/common/users) handler package with user registration, password reset, etc. Support oauth integrations for Google, Facebook, TikTok, etc.

5. Add new sections/tenant/profile handler with PUT and GET for TenantProfileData

6. Add new sections/tenant/account handler with basic and premium credit count, domainRegistered, payedAccount bool, etc.

7. Add new sections/tenant/domains handler with registered domain, domain search, existing domain support, etc.

8. Add new sections/tenant/filesystem handler which accepts a JSON blob via PUT and GET

9. Add multi-tenant models for each required object.
