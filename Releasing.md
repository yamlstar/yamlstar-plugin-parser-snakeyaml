# Releasing the SnakeYAML parser

## Order and prerequisites

1. Publish `org.yamlstar:snakeyaml-engine:3.2.0` from `yaml/snakeyaml-engine`.
2. Publish this plugin's Java, native, and Go artifacts as version `0.1.0`.
3. Release consumer changes pinned to those versions.

Verify ownership of `org.yamlstar` in the Central Portal before uploading.
The release helper reads `java.token` (falling back to `central.token`),
`gpg.key-id`, and the optional `gpg.passphrase` from
`~/.yamlstar-secrets.yaml`, using the same format as YAMLStar.
The signing key must be available to GPG.
No permanent Maven settings file or Maven-home override is needed.
Use the same version in the POM, Makefile, Java `VERSION`, C manifest,
`plugin.edn`, and Go `Version`.
The unprefixed release tag `0.1.0` and Go module tag `v0.1.0` must identify
the same reviewed commit.

## Local checks and signed bundle

```bash
make test test-goyaml-race test-graal-host test-goyaml-cli shellcheck
make bundle
```

`bundle` signs and stages the JAR, sources, Javadoc, and POM, then creates
`target/central-publishing/central-bundle.zip` without uploading it.
It uses the Central plugin's
[skipUpload option](https://central.sonatype.org/publish/publish-portal-maven/#skipupload).
The main JAR contains only the host-independent Java API.
GraalVM build classes and SDK dependencies are confined to the native profile.

Run this command yourself to upload the Java release for Central validation:

```bash
make release v=0.1.0
```

Run it from a clean checkout containing the reviewed release commit.
It verifies version alignment, builds and tests the local native archive,
creates matching annotated local tags `0.1.0` and `v0.1.0`, then uploads the
signed Java bundle.
Existing tags must already point to that commit.
The command does not publish GitHub tags or create a GitHub release.

`CENTRAL_TOKEN`, `GPG_KEY_ID`, and `GPG_PASSPHRASE` environment variables can
override file values.
`YS_SECRETS_FILE` selects an alternative secrets file.
Credentials are passed to Maven through environment variables and temporary
settings containing only variable references.

Automatic publication is disabled.
Review the deployment in the Central Portal and publish it there.
Verify that the main JAR, sources, Javadoc, POM, and signatures are downloadable.

## Native archives

Run the `Native archives` workflow manually at the reviewed release commit.
Select the branch or ref containing that commit and enter version `0.1.0`.
The workflow checks out the commit selected when the run starts and verifies
all version declarations without requiring release tags.
Check the workflow's recorded commit against the reviewed release commit.
It builds and tests Linux x64, Linux ARM64, and macOS ARM64 independently,
including loading from another GraalVM executable.
It uploads all archives and a combined `SHA256SUMS` as workflow artifacts.
No GitHub release is created automatically.

Download the combined `native-release` artifact to
`.cache/release-assets/0.1.0/`.
Keep it separate from `dist/`, since local builds replace `dist/SHA256SUMS`
with checksums for the current platform only.
Require both the Java 11 test job and the Linux native test job to pass at the
same reviewed commit before publication.

After publishing the Java deployment in Central, create a GitHub release
yourself using tag `0.1.0` at that commit.
Attach the three CI-built `.tar.xz` files and their combined `SHA256SUMS`.
For one platform, `make dist` produces the corresponding archive locally.
Release filenames are:

```text
yamlstar-plugin-parser-snakeyaml-0.1.0-linux-x64.tar.xz
yamlstar-plugin-parser-snakeyaml-0.1.0-linux-aarch64.tar.xz
yamlstar-plugin-parser-snakeyaml-0.1.0-macos-arm64.tar.xz
SHA256SUMS
```

Verify unauthenticated release downloads and checksums before making the Go
tag `v0.1.0` public at the same commit.
This ensures automatic native installation works when the Go module appears.
The Go adapter downloads from the unprefixed release tag and requires native
manifest version `0.1.0` to match Go `Version`.
The Go package is published through the matching `v0.1.0` module tag.
macOS Intel uses the Java artifact; no native archive is produced for it.
