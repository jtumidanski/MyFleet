GO_MODULES := $(shell go work edit -json | python3 -c "import json,sys;[print(m['DiskPath']) for m in json.load(sys.stdin)['Use']]" 2>/dev/null)

.PHONY: build test vet tidy fe-build fe-test up down lint ci

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

up:
	docker compose -f deploy/compose/docker-compose.yml up -d --build

down:
	docker compose -f deploy/compose/docker-compose.yml down -v

ci: vet test build fe-test fe-build
