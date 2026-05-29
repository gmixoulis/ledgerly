# Ledgerly

A REST microservice for managing companies, built in Go.

## Tech stack

| Concern | Choice |
|---------|--------|
| Router | [chi](https://github.com/go-chi/chi) |
| Database | PostgreSQL via [pgx v5](https://github.com/jackc/pgx) |
| Auth | JWT (HS256) via [golang-jwt](https://github.com/golang-jwt/jwt) |
| Events | Kafka via [kafka-go](https://github.com/segmentio/kafka-go) |
| Config | [Viper](https://github.com/spf13/viper) |
| Validation | [go-playground/validator](https://github.com/go-playground/validator) |
| Linter | [golangci-lint](https://golangci-lint.run) |

---

## Quick start with Docker Compose

```bash
# Build and start everything (Postgres + Kafka + app)
docker compose up --build

# The API is now available at http://localhost:8080
```

The `migrations/001_create_companies.sql` file is automatically applied by Postgres on first start.

---

## Local development (without Docker)

### Prerequisites

- Go 1.22+
- PostgreSQL 15+
- Kafka (optional; event failures are non-fatal)

### 1. Create the database

```sql
CREATE DATABASE companies;
```

Then run the migration:

```bash
psql -U postgres -d companies -f migrations/001_create_companies.sql
```

### 2. Configure

Edit `config.yaml` to match your local setup (DSN, Kafka brokers, JWT secret).  
You can also use environment variables (they take precedence over the file):

| Env var | Overrides |
|---------|-----------|
| `DATABASE_DSN` | `database.dsn` |
| `KAFKA_BROKERS` | `kafka.brokers` (comma-separated) |
| `JWT_SECRET` | `jwt.secret` |

### 3. Run

```bash
go run ./cmd/api
```

---

## API reference

### Authentication

All mutating endpoints (`POST`, `PATCH`, `DELETE`) require a `Bearer` token.

#### Login

```
POST /auth/login
Content-Type: application/json

{ "username": "admin", "password": "password" }
```

Response:

```json
{ "token": "<jwt>" }
```

Use the token in subsequent requests:

```
Authorization: Bearer <jwt>
```

---

### Companies

#### Create

```
POST /api/v1/companies
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Acme Corp",
  "description": "Optional, up to 3000 chars",
  "amount_employees": 42,
  "registered": true,
  "type": "Corporations"
}
```

Valid types: `Corporations`, `NonProfit`, `Cooperative`, `Sole Proprietorship`

#### Get

```
GET /api/v1/companies/{id}
```

#### Patch (partial update)

```
PATCH /api/v1/companies/{id}
Authorization: Bearer <token>
Content-Type: application/json

{ "amount_employees": 100 }
```

Only send the fields you want to change.

#### Delete

```
DELETE /api/v1/companies/{id}
Authorization: Bearer <token>
```

---

## Integration tests

The integration tests spin up the full HTTP stack against a real Postgres database.

```bash
# With the default DSN (postgres://postgres:postgres@localhost:5432/companies_test)
go test ./tests/integration/... -v

# Or point at a custom database
TEST_DATABASE_DSN="postgres://user:pass@host/dbname?sslmode=disable" \
  go test ./tests/integration/... -v
```

If Postgres is unreachable the tests are **skipped**, not failed, so they don't break CI when the DB isn't available.

---

## Linting

```bash
# Install golangci-lint (first time only)
brew install golangci-lint   # macOS
# or: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

golangci-lint run ./...
```

---

## Kafka events

On every mutating operation the service publishes a JSON message to the `company-events` topic (configurable).

| Operation | Event type |
|-----------|-----------|
| Create | `company.created` |
| Patch | `company.updated` |
| Delete | `company.deleted` |

Example payload:

```json
{
  "type": "company.created",
  "payload": { "id": "...", "name": "Acme", ... },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

If Kafka is unavailable the HTTP response is still returned successfully; the failure is only logged.
