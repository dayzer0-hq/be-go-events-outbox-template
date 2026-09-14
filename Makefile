DATABASE_URL ?= postgres://app:app@localhost:5432/app?sslmode=disable

# migrate applies every .sql file in migrations/ in name order.
#
# Plain psql inside the compose container, deliberately: it needs no extra tool
# installed and no migration framework to learn on day one. Files are applied in
# lexical order, so name them 0001_, 0002_ and so on.
.PHONY: migrate
migrate:
	@for f in migrations/*.sql; do \
		echo "applying $$f"; \
		docker compose exec -T db psql -v ON_ERROR_STOP=1 -U app -d app < "$$f" || exit 1; \
	done
	@echo "migrations applied"

.PHONY: test
test:
	go test ./...

.PHONY: run
run:
	go run ./cmd/api
