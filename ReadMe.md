# SnakeYAML parser plugin

One Java parser supplies YAMLStar, YAMLScript, and native ABI 2 consumers.
It depends on `org.yamlstar:snakeyaml-engine:3.2.0` and has no dependency on
either host application.

## Java

```xml
<dependency>
  <groupId>org.yamlstar</groupId>
  <artifactId>yamlstar-plugin-parser-snakeyaml</artifactId>
  <version>0.1.0</version>
</dependency>
```

```java
import org.yamlstar.snakeyaml.SnakeYamlParser;

var events = SnakeYamlParser.parse("answer: 42");
```

`parse` returns an immutable `List<ParseEvent>`.
Events expose named getters for their type, value, style, anchors, alias name,
tag, flow style, document directives, explicit boundaries, and source marks.
Collections in events are immutable.
Fields that do not apply are null, empty, or false.
Scalar styles are empty for plain scalars, or `single`, `double`, `literal`,
and `folded`.
Source lines, columns, and indices are zero-based; columns and indices count
Unicode code points, including supplementary characters as one code point.
Input is never rewritten or given an additional newline.
Malformed input throws an Engine exception with the original source marks.
Each call owns its parser, so calls may run concurrently.
Java 11 and newer are supported, including macOS Intel.

## Native consumers

The library exports the existing YAMLStar ABI 2 functions in
`include/yamlstar_plugin.h`.
Successful transforms return a UTF-8 EDN event vector.
Failures return nonzero status and UTF-8 diagnostic text when allocation
succeeds.
Every returned buffer belongs to the caller and must be released exactly once
with `yamlstar_plugin_v2_free`, including error buffers.
Lengths are authoritative; buffers are not NUL-terminated.
Freeing null is safe.
Transform accepts empty options, `nil`, or `{}`; other options are rejected.

The event format is `yamlstar-events-edn-v1` with additive `:start-mark`,
`:end-mark`, and `:tags` metadata.
Marks use the Java API's zero-based code-point coordinates.
The C shim initializes a plugin-owned GraalVM isolate once and attaches and
detaches calling threads as needed.
Isolate C API names are prefixed to avoid collisions with a GraalVM host.
Small native-only Java entry points expose the C shim's functions through
Native Image's export list without accessing Java state before attachment.
These use GraalVM 25's internal entry-point annotations, confined to the
native build profile.
Keep loaded libraries resident for the process lifetime.

Supported native release platforms are `linux-x64`, `linux-aarch64`, and
`macos-arm64`.
The shared library embeds its Java runtime and needs no installed JVM.
Native archives include the plugin manifest, ABI header, and licenses.
They retain the Engine's Apache 2.0 license and the GraalVM distribution's
legal notices alongside this plugin's MIT license.

## go-yaml

```go
import (
    "go.yaml.in/yaml/v4"
    "github.com/yamlstar/yamlstar-plugin-parser-snakeyaml/goyaml"
)

parser := goyaml.New(goyaml.WithInstall(false))
var value any
err := yaml.Load([]byte("answer: 42"), &value, yaml.WithPlugin(parser))
```

`goyaml.Register()` registers `yaml-parser=snakeyaml` without loading libraries
or downloading anything.
`New` enables installation by default; `WithInstall(false)` disables it.
The adapter loads on first parse and checks ABI, manifest, and exact version.
Installation verifies `SHA256SUMS` and does not replace an existing library.
`YAMLSTAR_LIBRARY_PATH` overrides standard library directories, including
when explicitly empty.
The adapter translates source lines and columns to go-yaml's one-based
convention while retaining code-point indices and document tag directives.
Importing the adapter leaves the default parser unchanged.

Build a configured go-yaml CLI:

```bash
make cli \
  PLUGIN-PACKAGE=github.com/yamlstar/yamlstar-plugin-parser-snakeyaml/goyaml@v0.1.0 \
  PLUGIN=yaml-parser=snakeyaml
```

Omit `PLUGIN` to register the adapter while keeping native go-yaml as default.

## Development

Makes installs tools under `.cache/`.
Install the Engine fork in the same project-managed Maven repository first.
In the coordinated YAMLStar checkout, run `make install-snakeyaml`.
For separate checkouts, pass the same absolute `MAKES_LOCAL_DIR` to both
projects so they share their local Maven repository.

```bash
make test
make test-build-order
make test-goyaml-race
make test-graal-host
make test-goyaml-cli
make shellcheck
make dist
```

`test-goyaml-cli` uses the sibling `../go-yaml` checkout by default, or
`GO_YAML_DIR`, and stages disposable sources and a local module proxy under
`.cache/integration/`.
It validates the generic package mechanism without publishing an artifact.
Java/native parity checks compare complete EDN vectors, including marks.
`test-build-order` builds the JVM code after the native code, then relinks and
exercises the native ABI to verify that their build outputs remain independent.
See [Releasing.md](Releasing.md) for release order and commands.
