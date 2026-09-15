.PHONY: build install frontend check dev-frontend clean

.PHONY: setup verify
setup:
	git config core.hooksPath .githooks

verify:
	# Require installed frontend dependencies; keep installation out of verification.
	test -d frontend/node_modules || { echo 'Run make install first'; exit 1; }
	# Reject unformatted Go source without modifying files.
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './frontend/node_modules/*'))"
	# Check Svelte and TypeScript diagnostics; the production build does not type-check.
	cd frontend && bun run check
	# Generate static assets before Go compiles the package that embeds them.
	cd frontend && bun run build
	# Run Go tests across all packages (currently there are no test files).
	go test ./...
	# Check Go source for suspicious constructs beyond compilation errors.
	go vet ./...
	# Create the output directory for the deployable executable.
	mkdir -p bin
	# Embed the frontend in a static Go binary and omit local source paths.
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

build: frontend
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/mega-agents .

install:
	cd frontend && bun install --frozen-lockfile

frontend: install
	cd frontend && bun run build

check:
	cd frontend && bun run check
	go vet ./...

dev-frontend:
	cd frontend && bun run dev

clean:
	go clean
