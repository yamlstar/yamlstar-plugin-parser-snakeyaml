# Releasing the SnakeYAML parser

## Prerequisites

Publish `org.yamlstar:snakeyaml-engine:3.2.0` before releasing this plugin.
The plugin release must be a reviewed, clean commit available on GitHub.
Make the reviewed source commit available there before running `make release`.
The command publishes artifacts and release tags from that source commit.
If GitHub reports that the commit is missing, no release publication has
started; make the commit available and rerun the command.
Use the same version in the POM, Makefile, Java `VERSION`, C manifest,
`plugin.edn`, and Go `Version`.

The release helpers read these fields from `~/.yamlstar-secrets.yaml`:

- `java.token`, falling back to `central.token`: the base64-encoded Central
  Portal `username:password` token.
- `gpg.key-id` and optional `gpg.passphrase`: the signing key and passphrase.
- `github.token`: a GitHub token with permission to create releases and tags,
  read workflow artifacts, and dispatch workflows.

The signing key must be available to GPG.
No permanent Maven settings file or Maven-home override is needed.
`CENTRAL_TOKEN`, `GPG_KEY_ID`, and `GPG_PASSPHRASE` override file values.
`GH_TOKEN` or `GITHUB_TOKEN` overrides the GitHub token.
An existing authenticated GitHub CLI can also be used.
`YS_SECRETS_FILE` selects an alternative secrets file.
Credentials are passed through environment variables and temporary Maven
settings containing only variable references.

## Publish the complete release

From the reviewed release checkout, run:

```bash
make release v=0.1.0
```

The command completes these steps in order:

1. Verify version declarations and a clean checkout, and reject conflicting
   public release tags.
2. Require successful `Test` and `Native archives` runs at that exact commit.
   Reuse successful runs, wait for active runs, or dispatch missing runs.
   Dispatching requires the current branch on GitHub to match local `HEAD`.
3. Download the combined `native-release` artifact and verify all three
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

If a step fails, rerun the same command from the same release commit.
It reuses an existing draft or published GitHub release and skips the Central
upload when the version's POM is already public.
It verifies public files before finishing any remaining tag publication.
A Central validation or publication failure leaves the GitHub release as a
draft and the Go tag unpublished.

## Native archives

The release command uses CI archives for Linux x64, Linux ARM64, and macOS
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

The matching workflow run and commit are recorded in
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

`test-release` checks credentials with fixture values and simulates publishing
services to exercise ordering, retries, CI failures, and conflicting tags.
It does not publish artifacts or tags.

`bundle` signs and stages the JAR, sources, Javadoc, and POM, then creates
`target/central-publishing/central-bundle.zip` without uploading it.
It uses the Central plugin's
[skipUpload option](https://central.sonatype.org/publish/publish-portal-maven/#skipupload).
The main JAR contains only the host-independent Java API.
GraalVM build classes and SDK dependencies are confined to the native profile.

Release consumer changes after this plugin is published.
