GO_MODULES := $(shell go work edit -json | python3 -c "import json,sys;[print(m['DiskPath']) for m in json.load(sys.stdin)['Use']]" 2>/dev/null)

.PHONY: build test vet tidy fe-build fe-test up down lint lint-check fmt ci

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

fe-test:
	npm run -w apps/web test

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

ci: lint-check vet test build fe-test fe-build
