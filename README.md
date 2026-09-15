# Mega Agents

## Local checks

Run `make install` once, then `make setup` to enable the tracked pre-commit
hook. Go, Bun, Make, and Gitleaks must be installed. Run `make verify` for
local type checks, Go tests, formatting checks, and the complete embedded build.
The hook scans staged changes with redacted Gitleaks output and checks an
isolated copy of staged files. It does not install dependencies or contact CI.
After dependency changes, run `make install` and stage the manifest and lockfile.
GitHub is not required for these local checks.

## Remote CI

Pull requests and pushes to main run two independent jobs: **Verify** repeats
local verification, and **Security** runs redacted secret scanning plus Bun and
Go vulnerability checks. Security also runs weekly over the full Git history
and current dependencies. Actions and tool versions are pinned; Dependabot
proposes weekly GitHub Actions updates. Both jobs must pass before merging.

Browser, race, and mutation suites will be added when the corresponding tests
exist. No placeholder passing jobs stand in for those tests.

The Svelte frontend uses Bun for dependency management and scripts. It is built
into `internal/web/dist`, then embedded into the Go executable with `go:embed`.

```sh
make build
./bin/mega-agents
```

Open <http://localhost:8080>. The JSON backend endpoint is available at
<http://localhost:8080/api/status>.

Set `PORT` at runtime if port 8080 is unavailable, for example
`PORT=3000 ./bin/mega-agents`.

For frontend development with hot reload:

```sh
make dev-frontend
```

For the Go server during development, run `make build` again whenever frontend
assets change, then restart `./bin/mega-agents`.
