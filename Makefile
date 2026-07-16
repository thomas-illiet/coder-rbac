SHELL_SCRIPTS := $(shell find dev -type f -name '*.sh' -print)

.PHONY: build test test-race vet fmt-check staticcheck vulncheck shellcheck docs-check helm-check compose-check quality check

build:
	go build -o bin/coder-rbac ./cmd/coder-rbac

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

staticcheck:
	go tool staticcheck -checks=all ./...

vulncheck:
	go tool govulncheck ./...

shellcheck:
	shellcheck $(SHELL_SCRIPTS)

node_modules/.package-lock.json: package-lock.json
	npm ci --ignore-scripts --no-audit --no-fund

docs-check: node_modules/.package-lock.json
	npm run docs:lint
	go test ./internal/quality -count=1

helm-check:
	helm lint charts/coder-rbac -f charts/coder-rbac/ci/values.yaml
	helm template coder-rbac charts/coder-rbac -f charts/coder-rbac/ci/values.yaml >/dev/null
	@if helm template coder-rbac charts/coder-rbac -f charts/coder-rbac/ci/values.yaml \
		--set-string 'config.allowedUserIDs[0]=legacy' >/dev/null 2>&1; then \
		echo "legacy config.allowedUserIDs was unexpectedly accepted" >&2; \
		exit 1; \
	fi

compose-check:
	docker compose config --quiet

quality: fmt-check vet staticcheck vulncheck shellcheck docs-check helm-check compose-check

check: quality test-race
