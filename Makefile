GO_TOOL_BIN := $(shell go env GOPATH)/bin
STATICCHECK := $(GO_TOOL_BIN)/staticcheck
ACTIONLINT := $(GO_TOOL_BIN)/actionlint

.PHONY: build install frontend check dev-frontend clean

.PHONY: setup verify
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
	# Require installed frontend dependencies; keep installation out of verification.
	test -d frontend/node_modules || { echo 'Run make install first'; exit 1; }
	test -x $(STATICCHECK) || { echo 'Run make install to install staticcheck'; exit 1; }
	test -x $(ACTIONLINT) || { echo 'Run make install to install actionlint'; exit 1; }
	# Cap local concurrency so parallel agent work does not saturate the machine.
	$(MAKE) -j2 --output-sync=target frontend-quality frontend
	$(MAKE) -j2 --output-sync=target go-quality workflow-check test-policy
	$(MAKE) binary size-check

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
	go test -timeout=30s $$(go list ./... | rg -v '/frontend/node_modules/')
	go test -timeout=30s -coverprofile=reports/go-coverage.out ./internal/...
	go tool cover -func=reports/go-coverage.out | awk '/^total:/ { value=$$3 + 0; if (value < 80) { print "Go coverage " value "% is below 80%"; exit 1 } }'

go-vet:
	go vet $$(go list ./... | rg -v '/frontend/node_modules/')

go-staticcheck:
	$(STATICCHECK) $$(go list ./... | rg -v '/frontend/node_modules/')

workflow-check:
	$(ACTIONLINT) .github/workflows/*.yml

test-policy:
	bash scripts/check-test-policy.sh .

binary:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

size-check:
	bash scripts/check-artifact-sizes.sh

gate-self-test:
	bash scripts/test-gates.sh

smoke: build
	bash scripts/smoke.sh

build: frontend
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

install:
	cd frontend && bun install --frozen-lockfile
	@test -x $(STATICCHECK) || go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
	@test -x $(ACTIONLINT) || go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.7

frontend: install
	cd frontend && bun run build

check:
	cd frontend && bun run check
	go vet $$(go list ./... | rg -v '/frontend/node_modules/')

dev-frontend:
	cd frontend && bun run dev

clean:
	go clean
