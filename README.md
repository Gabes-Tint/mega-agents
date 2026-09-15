# Mega Agents

## Local checks

Run `make install` once, then `make setup` to enable the tracked pre-commit
hook. Go, Bun, Make, and Gitleaks must be installed. Run `make verify` for
local type checks, Go tests, formatting checks, and the complete embedded build.
The hook scans staged changes with redacted Gitleaks output and checks an
isolated copy of staged files. It does not install dependencies or contact CI.
The hook delegates to `make pre-commit`, which runs `secrets-staged`,
`whitespace-staged`, and `verify-staged` in order. The snapshot helper lives in
`scripts/verify-staged.sh` and calls `make verify` on the staged copy.
After dependency changes, run `make install` and stage the manifest and lockfile.
GitHub is not required for these local checks.

## Remote CI

Pull requests and pushes to main run two independent deterministic jobs:
**Verify** repeats local verification, and **Security** runs redacted secret
scanning. Actions and tool versions are pinned; both jobs must pass before
merging.

The separate **Dependency audit** workflow runs weekly and on manual request.
It checks the current Bun and Go vulnerability feeds. Those findings remain
visible without making an unchanged commit fail its merge checks because an
external advisory database changed. Dependabot separately proposes weekly
GitHub Actions updates.

CI caches Go compilation and modules, Bun package downloads, and pinned security
tool binaries. Scan results and generated frontend assets are not cached.
Verify and Security run concurrently, and superseded runs are cancelled.
Measure cold and warm GitHub runs before adding more jobs: extra runners also
add setup costs. Tool-version changes invalidate the security binary cache.

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
