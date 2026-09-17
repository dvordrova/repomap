# Echo sqlc service

A small standalone Go repository for repository-analysis tests. It has two
executable targets: an Echo API exposing `GET /users/:id` and a Cobra CLI
exposing `users get --id ID`. Both targets use the same application wiring and
follow this flow:

1. The Echo handler calls the user service through an interface.
2. The service calls the user repository through an interface.
3. The repository calls sqlc-generated PostgreSQL queries.
4. Goverter-generated code maps the domain model to the HTTP response.

The first migration creates `users` with only `id`; the second migration adds
`name`.

```sh
go generate ./internal/users/handler
sqlc generate
DATABASE_URL=postgres://postgres:postgres@localhost:5432/example?sslmode=disable go run ./cmd/api
DATABASE_URL=postgres://postgres:postgres@localhost:5432/example?sslmode=disable go run ./cmd/users get --id 1
```
