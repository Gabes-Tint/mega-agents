GO_TOOL_BIN := $(shell go env GOPATH)/bin
STATICCHECK := $(GO_TOOL_BIN)/staticcheck
ACTIONLINT := $(GO_TOOL_BIN)/actionlint
AIR := $(GO_TOOL_BIN)/air

.PHONY: build install frontend check dev dev-frontend clean agent-eval

.PHONY: setup verify gates
.PHONY: pre-commit secrets-staged whitespace-staged verify-staged
# Run checks in sequence, including when the caller enables parallel Make.
pre-commit:
	$(MAKE) secrets-staged
	$(MAKE) whitespace-staged
	$(MAKE) verify-staged

secrets-staged:
	@command -v gitleaks >/dev/null || { echo 'Install gitleaks before committing.' >&2; exit 1; }
	gitleaks git --pre-commit --staged --redact --no-banner --ignore-gitleaks-allow .

whitespace-staged:
	git diff --cached --check

# The helper isolates staged files, then runs the normal verification recipe.
verify-staged:
	bash scripts/verify-staged.sh

setup:
	git config core.hooksPath .githooks

verify:
	@scripts/run-with-status.sh 'Verification prerequisites' sh -c \
		'test -d frontend/node_modules && test -x "$(STATICCHECK)" && test -x "$(ACTIONLINT)"' || \
		{ echo 'Run make install first' >&2; exit 1; }
	@scripts/run-with-status.sh 'Frontend checks' \
		$(MAKE) -j2 --output-sync=target frontend-quality frontend
	@scripts/run-with-status.sh 'Go and workflow checks' \
		$(MAKE) -j2 --output-sync=target go-quality workflow-check
	@scripts/run-with-status.sh 'Repository gates' $(MAKE) gates

frontend-quality: frontend-format frontend-check frontend-lint frontend-test

frontend-format:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './frontend/node_modules/*'))"
	cd frontend && bun run format:check

frontend-check:
	cd frontend && bun run check

frontend-lint:
	cd frontend && bun run lint

frontend-test:
	cd frontend && bun run test:coverage

go-quality: go-test go-vet go-staticcheck

go-test:
	mkdir -p reports
	go test -timeout=30s $$(go list ./... | grep -v '/frontend/node_modules/')
	go test -timeout=30s -coverprofile=reports/go-coverage.out ./internal/...
	go tool cover -func=reports/go-coverage.out | awk '/^total:/ { value=$$3 + 0; if (value < 80) { print "Go coverage " value "% is below 80%"; exit 1 } }'

go-vet:
	go vet $$(go list ./... | grep -v '/frontend/node_modules/')

go-staticcheck:
	$(STATICCHECK) $$(go list ./... | grep -v '/frontend/node_modules/')

workflow-check:
	$(ACTIONLINT) .github/workflows/*.yml

test-policy:
	bash scripts/gates/check-test-policy.sh .

agent-adapters:
	bash scripts/gates/check-agent-adapters.sh .

ci-documentation:
	bash scripts/gates/check-ci-documentation.sh .

gates: binary test-policy agent-adapters ci-documentation size-check

binary:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

size-check:
	bash scripts/gates/check-artifact-sizes.sh

gate-self-test:
	bash scripts/test-gates.sh
	bash scripts/test-run-with-status.sh

smoke: build
	bash scripts/smoke.sh

build: frontend
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

install:
	cd frontend && bun install --frozen-lockfile
	@test -x $(STATICCHECK) || go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
	@test -x $(ACTIONLINT) || go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
	@test -x $(AIR) || go install github.com/air-verse/air@v1.67.4

frontend: install
	cd frontend && bun run build

check:
	cd frontend && bun run check
	go vet $$(go list ./... | grep -v '/frontend/node_modules/')

dev-frontend:
	cd frontend && bun run dev

dev:
	bash scripts/dev.sh

clean:
	go clean

TOOL ?= opencode
OPENCODE ?= opencode
RESULT ?= reports/agent-eval/result.json

agent-eval:
	go run ./cmd/agent-eval --tool "$(TOOL)" --model "$(MODEL)" --variant "$(VARIANT)" --executable "$(OPENCODE)" --output "$(RESULT)"
