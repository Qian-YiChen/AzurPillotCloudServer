# Repository Guidelines

## Project Structure & Module Organization

Go 1.26/PostgreSQL backend with an embedded React admin UI.

- `cmd/server/`: startup, dependency wiring, and shutdown.
- `internal/`: configuration, gateway protocols, caching, HTTP handlers, storage, publishing, and UI embedding. Keep logic in the corresponding package.
- `internal/store/migrations/`: embedded SQL migrations, named like `0001_init.sql`.
- `web/src/`: React/TypeScript components, views, API client, and CSS.
- `workers/`: static Worker templates; `public/`: generated announcement/update JSON assets.
- Go `*_test.go` files sit beside sources; CI lives in `.github/workflows/`.

## Build, Test, and Development Commands

Use Node.js 22 and PostgreSQL. Run from the repository root:

- `npm --prefix web ci`: install locked frontend dependencies.
- `npm --prefix web run dev`: start Vite on port 5173, proxying `/api` to port 8080.
- `npm --prefix web run build`: type-check and build into `internal/adminui/dist/`.
- `make all`: build frontend assets, then the server in `bin/`; install dependencies first.
- `go run ./cmd/server`: start after building frontend assets; `go run -tags noadminui ./cmd/server` omits the UI.
- `docker compose up -d --build`: build and run the server with PostgreSQL.
- `go test -race -count=1 -cover ./...`: run tests with race detection and coverage.
- `go vet ./...` and `golangci-lint run`: run backend static checks.

## Coding Style & Naming Conventions

Format Go with `gofmt` (tabs); follow `.golangci.yml`. Use lowercase package names. Preserve the standard-library HTTP/pgx architecture.

Use two-space indentation, single quotes, and no semicolons in TypeScript. Name components/views in PascalCase, e.g. `MaintenanceView.tsx`. TypeScript uses strict checks. Read `DESIGN.md` before UI changes.

## Testing Guidelines

Use Go's `testing`, `httptest`, and `goleak`. Name tests `TestBehavior`; cover validation, failures, and concurrency. Set `TEST_DATABASE_URL` to a dedicated PostgreSQL database; integration tests skip when unset. CI reports coverage without a numeric threshold. For frontend changes, build and manually verify affected `/admin/` flows; no frontend test runner exists.

## Commit & Pull Request Guidelines

History uses `feat:` and `fix:` prefixes with concise, mostly Chinese descriptions; use `docs:` for documentation. After completing changes and necessary validation, create a focused local commit unless explicitly instructed otherwise.

PRs should explain changes, link relevant issues, and record validation results and database-test skips. Include UI screenshots and configuration/migration notes when applicable.

## Security & Configuration

Consult `.env.example`; direct Go runs require environment variables to be loaded explicitly. Keep tokens server-side and out of Git. Empty `ADMIN_TOKEN` disables admin APIs; omitting Cloudflare credentials keeps publishing local. Do not commit generated frontend bundles.
