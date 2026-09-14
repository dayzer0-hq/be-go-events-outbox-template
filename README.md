# Go + Postgres + Redis Streams — API and worker — starting point

A runnable Go service and a companion worker, backed by Postgres and Redis Streams. **Everything your brief asks you to build is missing on purpose** — this is a starting point,
not a partial answer.

## Run it

You need **git**, **Go 1.22 or newer**, and **Docker**.

```bash
docker compose up -d
make migrate
go test ./...          # green on the untouched skeleton
go run ./cmd/api       # then, in another terminal:
curl localhost:8080/healthz
```

Then, in a third terminal:

```bash
go run ./cmd/worker
```

It reports the stream it can address and then waits. It consumes nothing yet — that is ticket 2
onwards.

`/healthz` answers JSON naming each dependency. If it does, you are ready to start ticket 2.

If `go test ./...` is **red before you have changed anything**, that is worth telling Raj about
rather than working around — a skeleton that arrives broken is our bug, not yours.

## What is here

```
cmd/api/main.go          the entry point: connects to both, serves /healthz
cmd/worker/main.go       the consumer side: connects, reports, waits
internal/store/store.go  the pgx connection pool
internal/store/redis.go  the Redis client, and the Pinger adapter
internal/httpx/health.go the health handler
migrations/0001_init.sql the first migration — no application tables yet
docker-compose.yml       Postgres 16 and Redis 7, both with healthchecks
Makefile                 make migrate, make test, make run
```

Four decisions worth knowing rather than discovering:

- **`go test ./...` needs neither Postgres nor Redis.** `httpx.Health` takes `Pinger`
  interfaces, so the handler is tested against stubs. A skeleton whose tests require Docker to be
  up first is red for everybody who has not read ahead.
- **`/healthz` reports each dependency separately.** One collapsed boolean tells you something is
  wrong and not which, which is the question you actually have at 2am.
- **`store.RedisPinger` exists because go-redis returns `*StatusCmd` from `Ping`, not an
  error.** The adapter is there so `Pinger` can stay the smallest thing the handler needs — one
  method, satisfiable by a stub.
- **The compose healthchecks make `up -d` mean *ready*, not *started*.** Without them `make
  migrate` can hit a Postgres that is still booting and fail in a way that reads like a bad
  migration.

`DATABASE_URL`, `REDIS_ADDR` and `ADDR` override the defaults.

## What is NOT here

**Your brief's seeded data is not in this template, and neither is any harness it names** — the chaos harness, the replay tool, the PSP or provider stubs, the fleet simulator, the verifier.
This repository is shared by several briefs that each need different data and different tools, so
it carries the parts that are the same for all of them and none of the parts that are not.

If your brief's "What's provided" section describes a seeded database or a named helper you cannot
find here, **that is a real gap and not something you have missed**. Say so on the ticket: Raj
would rather hear it early than have you build against data you had to invent.

## Adding what you need

1. write your schema as `migrations/0002_<name>.sql` and run `make migrate`
2. load the seed data your brief describes
3. add handlers under `internal/httpx`, wiring them in `cmd/api/main.go`
4. test each one — `internal/httpx/health_test.go` shows the shape

Commit on a branch and open a pull request; do not commit on `main`.
