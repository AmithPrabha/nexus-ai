.PHONY: setup up down migrate smoke test lint
setup:
	python3 scripts/setup.py
up: setup
	docker compose up -d --build --wait
down:
	docker compose down
migrate:
	docker compose run --rm migrate
smoke:
	python3 scripts/smoke.py
test:
	cd services/gateway && go test ./...
	cd services/ai && uv run --frozen pytest -q
lint:
	cd services/gateway && go vet ./... && test -z "$$(gofmt -l *.go)"
	cd services/ai && uv run --frozen ruff check .

.PHONY: integration
integration:
	docker compose -f compose.yaml -f infra/checks.compose.yaml run --build --rm integration /checks/test_invoices.py

.PHONY: seed policy
seed:
	docker compose run --rm seed
policy:
	docker compose -f compose.yaml -f infra/checks.compose.yaml run --build --rm integration /checks/test_policy.py

.PHONY: security web-check
security:
	python3 scripts/scan_secrets.py
web-check:
	cd apps/web && npm ci && npm run lint && npm run typecheck && npm run contracts -- --check

.PHONY: eval
eval: setup
	python3 scripts/evaluate.py

.PHONY: sandbox sandbox-check
sandbox: setup
	docker compose -f compose.yaml -f infra/n8n.compose.yaml up -d --build --wait
sandbox-check:
	docker compose -f compose.yaml -f infra/checks.compose.yaml run --build --rm integration /checks/test_notifications.py

.PHONY: sandbox-down
sandbox-down:
	docker compose -f compose.yaml -f infra/n8n.compose.yaml down

.PHONY: tenant-check
tenant-check:
	docker compose -f compose.yaml -f infra/checks.compose.yaml run --build --rm integration /checks/test_tenants.py
