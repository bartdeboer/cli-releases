# CLI releases

Public binary releases for selected CLI tools maintained by Bart de Boer.

Each tool is released independently under a namespaced tag such as
`go-releaser/v0.1.0`. Release assets include platform archives,
`manifest.json`, and `SHA256SUMS.txt` for integrity verification.

The corresponding source repositories may be private. Publishing binaries
here does not make their source code open source or grant additional rights to
use, modify, or redistribute them.

## `cli-get`

### Install cli-get

Requires Go 1.24+ and `$GOBIN` (or `$(go env GOPATH)/bin`) on `PATH`.

```sh
go install github.com/bartdeboer/cli-releases/cmd/cli-get@latest
```

`cli-get` installs a selected published CLI from this repository without GitHub authentication:

```sh
cli-get install go-releaser --version v0.1.0
cli-get install go-releaser --version v0.1.0 --bin-dir "$HOME/go/bin" --json
cli-get install go-releaser --version v0.1.0 --overwrite
```

M1 supports Linux amd64 and arm64. The default destination is `$HOME/go/bin`. The directory must already exist, be owned by the invoking user, contain no symlink path components, and not be group/world writable. Existing tools are never replaced unless `--overwrite` is explicit.

The repository, API origins, release tag shape, asset names, and checksums are not configurable. The installer validates the canonical `cli-releases.manifest/v1` manifest, exact remote asset inventory, `SHA256SUMS.txt`, platform archive size and SHA-256, and an archive containing exactly one normalized executable. It downloads with fixed bounds and secure redirect rules, never invokes the downloaded program, and stages privately in the destination directory before atomic installation. It writes no workspace, authentication, or configuration state.

Source bootstrap is deliberately separate from remote installation:

```sh
go run ./cmd/cli-get bootstrap
go run ./cmd/cli-get bootstrap --source /workspace/src/cli-releases
```

A future Hostbridge alias should execute only `cli-get`, use an empty fixed argument prefix, and set `allow_extra_args=true`. Updating `cli-get` itself will use the same typed install command once a `cli-get` release exists.

### Trust and licensing

Integrity metadata and archives are served from the same public release, so SHA-256 protects against corruption and inconsistent assets but does not create an independent trust root. Trust is anchored in the GitHub repository and its publisher controls. Source repositories may remain private as described above.

No software license has been explicitly selected for this repository. No LICENSE file is added; public visibility alone does not grant reuse rights. Bart should choose a license before describing `cli-get` as open source or inviting redistribution.

Command routing and contextual help use `github.com/bartdeboer/go-clir` v0.3.0. Security-sensitive option parsing remains owned by `cli-get`. Duplicate flags are rejected uniformly; this intentionally hardens the original M1 parser, which accepted repeated booleans and used the last repeated value.
