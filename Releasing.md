# Releasing the SnakeYAML parser

## Prerequisites

Publish `org.yamlstar:snakeyaml-engine:3.2.0` before releasing this plugin.
The plugin release must be a reviewed, clean commit available on GitHub.
Make the reviewed source commit available there before running `make release`.
The workflow publishes artifacts and release tags from that source commit.
If GitHub reports that the commit is missing, no release publication has
started; make the commit available and rerun the command.
Use the same version in the POM, Makefile, Java `VERSION`, C manifest,
`plugin.edn`, and Go `Version`.
Bump these declarations before the next release.

## Credentials

The local command only needs GitHub authentication with permission to dispatch
and rerun Actions workflows.
It reads `github.token` from `~/.yamlstar-secrets.yaml`, or uses `GH_TOKEN`,
`GITHUB_TOKEN`, or existing GitHub CLI authentication.
`YS_SECRETS_FILE` selects an alternative secrets file.

Configure these repository or organization Actions secrets for CI publication:

- `CENTRAL_TOKEN`: the base64-encoded Portal `username:password` token,
  corresponding to `java.token` or `central.token` in the local secrets file.
- `GPG_KEY_ID`: the signing key ID, corresponding to `gpg.key-id`.
- `GPG_PRIVATE_KEY`: the ASCII-armored private signing key.
- `GPG_PASSPHRASE`: the optional signing passphrase, corresponding to
  `gpg.passphrase`.

If the organization already supplies these secrets, grant this repository
access to them.
The workflow imports the key into a temporary GPG directory and removes it
after publication.
It uses the automatic `GITHUB_TOKEN` to create GitHub releases and tags.
No separate GitHub publishing token is required in CI.
Maven credentials are passed through environment variables and temporary
settings containing only variable references.

## Start the release workflow

From the reviewed release checkout, run:

```bash
make release v=0.1.0
```

The command checks versions, the clean checkout, and the published source
commit, then dispatches the `Release` workflow and prints its Actions URL.
It returns after dispatching.
The terminal can be closed; all validation and publication continue in Actions.
No local signing or publishing runs after the workflow starts.

You can also start `Release` directly from the Actions page, selecting the
reviewed branch or ref and version.
The optional `commit` input must match the commit selected for that run.

The workflow completes these steps in order:

1. Pin and validate the source commit and version.
2. Run the reusable `Test` and `Native archives` workflows for that commit.
   Both must pass before the publishing job starts.
3. Download this run's combined `native-release` artifact and verify all three
   archives, checksums, manifest versions, architectures, and licenses.
4. Create a draft GitHub release `0.1.0` and upload the CI-built archives
   and their combined `SHA256SUMS`.
5. Sign and publish the Java artifacts to Central, wait for publication,
   and verify that the POM, JARs, and signatures are downloadable.
6. Publish the GitHub release and verify its public downloads against the
   CI files.
7. Publish the Go module tag `v0.1.0` at the same commit.

Central publication is automatic, using the Maven plugin's
[autoPublish and waitUntil options](https://central.sonatype.org/publish/publish-portal-maven/#wait-for-publishing).
There is no separate Portal Publish step.
The command creates GitHub tags through the API and never moves an existing
tag to a different commit.
The Go tag becomes public only after its automatic native downloads work.

If a step fails, use **Re-run failed jobs** in Actions or rerun `make release`
from the same release commit.
The local command restarts failed jobs and returns without waiting.
If a run is active or already successful, it prints that run's status and URL
without starting another release.
The publishing job reuses an existing draft or published GitHub release and
skips the Central upload when the version's POM is already public.
It verifies public files before finishing remaining tag publication.
A Central validation or publication failure leaves the GitHub release as a
draft and the Go tag unpublished.

## Native archives

The workflow builds CI archives for Linux x64, Linux ARM64, and macOS
ARM64, including tests that load from another GraalVM executable.
The `Native archives` workflow can also be run manually before releasing,
with version `0.1.0`, without creating release tags.

Validated files are saved under `.cache/release-assets/0.1.0/`:

```text
yamlstar-plugin-parser-snakeyaml-0.1.0-linux-x64.tar.xz
yamlstar-plugin-parser-snakeyaml-0.1.0-linux-aarch64.tar.xz
yamlstar-plugin-parser-snakeyaml-0.1.0-macos-arm64.tar.xz
SHA256SUMS
```

Inside the publishing runner, the workflow run and commit are recorded in
`.cache/release-assets/0.1.0-provenance.json`.
Keep these files separate from `dist/`, since local builds replace
`dist/SHA256SUMS` with checksums for the current platform only.
macOS Intel uses the Java artifact; no native archive is produced for it.

## Local validation and bundle

```bash
make test test-goyaml-race test-graal-host test-goyaml-cli shellcheck
make test-release
make bundle
```

`test-release` checks credentials with fixture values and simulates workflow
dispatch and publishing services to exercise ordering, retries, commit
validation, and conflicting tags.
It does not publish artifacts or tags.

`bundle` signs and stages the JAR, sources, Javadoc, and POM, then creates
`target/central-publishing/central-bundle.zip` without uploading it.
This optional local target reads `java.token` or `central.token`, `gpg.key-id`,
and optional `gpg.passphrase` from `~/.yamlstar-secrets.yaml`.
It also accepts `CENTRAL_TOKEN`, `GPG_KEY_ID`, and `GPG_PASSPHRASE` overrides.
It uses the Central plugin's
[skipUpload option](https://central.sonatype.org/publish/publish-portal-maven/#skipupload).
The main JAR contains only the host-independent Java API.
GraalVM build classes and SDK dependencies are confined to the native profile.

Release consumer changes after this plugin is published.
