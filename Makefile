GO_MODULES := $(shell go work edit -json | python3 -c "import json,sys;[print(m['DiskPath']) for m in json.load(sys.stdin)['Use']]" 2>/dev/null)

.PHONY: build test vet tidy fe-build fe-test up down lint lint-check fmt manifests carfax-template ci

build: ## go build every module in the workspace
	go build github.com/jtumidanski/myfleet/...

vet:
	go vet github.com/jtumidanski/myfleet/...

test:
	go test -race github.com/jtumidanski/myfleet/...

tidy:
	@for m in $(GO_MODULES); do (cd $$m && go mod tidy); done

fe-build:
	npm run -w apps/web build

# Every JS workspace that has tests, not just apps/web. packages/shared-ts owns
# fetchAuthenticated — the single 401-refresh path every SPA call goes through —
# and its tests previously ran in no automated gate at all. packages/ui-components
# was in the same position: it had a test script and a test file, and nothing ran
# them.
fe-test:
	npm run -w apps/web test
	npm run -w packages/shared-ts test
	npm run -w packages/ui-components test

lint: ## fix mode: formatters + auto-fixable lint findings, Go and web
	./tools/lint.sh

lint-check: ## check mode: mutate nothing, fail on any violation (what CI runs)
	./tools/lint.sh --check

fmt: ## formatter layer only
	./tools/lint.sh --fmt

up:
	docker compose -f deploy/compose/docker-compose.yml up -d --build

down:
	docker compose -f deploy/compose/docker-compose.yml down -v

manifests: ## render deploy/k8s overlays and assert their invariants
	./tools/check-manifests.sh

carfax-template: ## the Carfax URL template lives in 3 files; fail if they drift
	./tools/check-carfax-template.sh

ci: lint-check vet test build fe-test fe-build manifests carfax-template
