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

M1 supports Linux and macOS (Darwin) on amd64 and arm64. The default destination is `$HOME/go/bin`. The directory must already exist, be owned by the invoking user, contain no symlink path components, and not be group/world writable. Existing tools are never replaced unless `--overwrite` is explicit.

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

## Read verified release documentation

```text
cli-get readme TOOL [--version vX.Y.Z]
```

With a version, retrieve that exact tool/version. Without one, scan at most
10 pages of 100 public release records and select the highest canonical
stable SemVer for that tool (not GitHub's repository-wide latest release).
An incomplete scan fails and asks for an explicit version.

The command validates release identity, canonical manifest, exact declared
remote asset inventory and metadata, checksum file, and README size/SHA-256.
It downloads only release metadata, manifest.json, SHA256SUMS.txt and README.md.
It never downloads platform binaries or docs.zip, installs files, extracts
archives, renders Markdown, follows documentation links, or executes code.
UTF-8 README content is written as inert text; terminal controls and Unicode
format/bidi controls are escaped, except ordinary newline/tab. Missing README
is an explicit error. Markdown link text is preserved, not opened.

Manifest v1 remains supported. Documentation-bearing v2 manifests retain
all v1 fields then append `documentation: [{file, size, sha256}]`, declaring
required README.md and optional docs.zip in that order. README is at most
1 MiB; docs.zip is at most 32 MiB. Both are separate checksummed release
assets. Installation validates their inventory declarations but downloads
and installs only the selected executable archive; documentation is never
treated as an executable. Platform archive validation remains unchanged.

For offline documentation, README.md belongs beside the `docs/` tree
contained in docs.zip. README links should use `docs/<relative-path>`.
cli-get readme deliberately does not extract that tree or resolve links.

**Trust limit:** matching same-origin checksums are not signatures or proof
of a trustworthy publisher. Treat verified documentation as untrusted text.

Shared producer fixtures are copied byte-for-byte into
`internal/cliget/testdata/bundle-v1` and `bundle-v2`.
