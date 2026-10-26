package goyaml

import (
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.yaml.in/yaml/v4"
	"go.yaml.in/yaml/v4/plugin/loaderlimits"
)

func TestLibraryWithoutGoRuntime(t *testing.T) {
	path := filepath.Join("..", "lib", libraryFilename())
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if info, err := buildinfo.ReadFile(path); err == nil {
		t.Fatalf("native library still embeds Go: %s", info.GoVersion)
	}
}

func TestProfiledConcurrency(t *testing.T) {
	// Exercise foreign calls with aggressive Go heap sampling.
	// A subprocess applies the setting before any native library is loaded.
	t.Setenv("GODEBUG", "memprofilerate=1")
	command := exec.Command(os.Args[0], "-test.run=^TestConcurrentLoads$")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("profiled native calls: %v\n%s", err, output)
	}
}

func useTestLibrary(t *testing.T) {
	t.Helper()
	path, err := filepath.Abs("../lib")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", path)
}

func TestLoad(t *testing.T) {
	useTestLibrary(t)
	for _, input := range []string{
		"a: [1, true, null, \"\"]\nb: false\n",
		"base: &a [one, two]\ncopy: *a\n",
		"literal: |\n  line\nfolded: >-\n  more\n  words\n",
		"unicode: 'λ🙂'\nquoted: \"a\\0b\\n\\t\"\n",
		"--- true\n...\n--- false\n",
		"", "# comment\n",
	} {
		t.Run(input, func(t *testing.T) {
			var got, want []any
			if err := yaml.Load([]byte(input), &got, yaml.WithAllDocuments(),
				yaml.WithPlugin(New())); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Load([]byte(input), &want,
				yaml.WithAllDocuments()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v; want %#v", got, want)
			}
		})
	}
}

func TestMetadata(t *testing.T) {
	useTestLibrary(t)
	input := "%YAML 1.2\n%TAG !e! tag:example.com,2026:\n" +
		"--- &a !e!items [!!str 1, 'two', *a]\n...\n"
	events, err := New().Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 9 || events[0].Type != "stream_start" ||
		events[8].Type != "stream_end" || !events[1].Explicit ||
		!events[7].Explicit || events[1].Version == nil ||
		events[1].Version.Major != 1 || events[1].Version.Minor != 2 {
		t.Fatalf("document metadata: %+v", events)
	}
	if events[2].Anchor != "a" || events[2].Tag != "tag:example.com,2026:items" ||
		!events[2].Flow || events[3].Value != "1" ||
		events[3].Tag != "tag:yaml.org,2002:str" ||
		events[4].Style != "single" || events[4].Value != "two" ||
		events[5].Type != "alias" || events[5].Anchor != "a" {
		t.Fatalf("node metadata: %+v", events)
	}
	for _, event := range events {
		if event.StartMark.Line < 1 || event.StartMark.Column < 1 ||
			event.EndMark.Line < 1 || event.EndMark.Column < 1 {
			t.Fatalf("missing position: %+v", event)
		}
	}
}

func TestRegistration(t *testing.T) {
	useTestLibrary(t)
	if os.Getenv("RAPIDYAML_REGISTER_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegistration$")
		cmd.Env = append(os.Environ(), "RAPIDYAML_REGISTER_TEST=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("registration subprocess: %v\n%s", err, output)
		}
		return
	}
	if _, err := yaml.OptsYAML("plugin: {yaml-parser: snakeyaml}"); err == nil {
		t.Fatal("import registered the adapter")
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", t.TempDir())
	if err := Register(); err != nil {
		t.Fatal(err)
	}
	opt, err := yaml.OptsYAML(
		"plugin: {yaml-parser: {name: snakeyaml, install: false}}")
	if err != nil {
		t.Fatal(err)
	}
	var missing any
	if err := yaml.Load([]byte("true"), &missing, opt); err == nil ||
		!strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("offline parse without a library: %v", err)
	}
	useTestLibrary(t)
	lib, err := loadLibrary()
	if err != nil {
		t.Fatal(err)
	}
	for _, config := range []string{
		"plugin: {yaml-parser: snakeyaml}",
		"plugin: {yaml-parser: snakeyaml@v" + lib.version + "}",
	} {
		opt, err := yaml.OptsYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		var got bool
		if err := yaml.Load([]byte("true"), &got, opt); err != nil || !got {
			t.Fatalf("got %v, %v", got, err)
		}
	}
	for _, config := range []string{
		"plugin: {yaml-parser: snakeyaml@v0.0.1}",
		"plugin: {yaml-parser: {name: snakeyaml, unknown: true}}",
		"plugin: {yaml-parser: {name: snakeyaml, install: nope}}",
	} {
		if _, err := yaml.OptsYAML(config); err == nil {
			t.Fatalf("accepted %q", config)
		}
	}
	for _, config := range []string{"{}", "plugin: {yaml-parser: true}"} {
		opt, err := yaml.OptsYAML(config)
		if err != nil {
			t.Fatalf("native default changed: %v", err)
		}
		var got string
		if err := yaml.Load([]byte("native"), &got, opt); err != nil ||
			got != "native" {
			t.Fatalf("native load: %q, %v", got, err)
		}
	}
	if err := Register(); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate registration: %v", err)
	}
}

func TestErrorsAndLimits(t *testing.T) {
	useTestLibrary(t)
	p := New()
	for _, input := range [][]byte{[]byte("[unterminated"), {0xff}} {
		if _, err := p.Parse(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	var got any
	if err := yaml.Load([]byte("[[[0]]]"), &got, yaml.WithPlugin(p),
		yaml.WithPlugin(loaderlimits.New(loaderlimits.DepthValue(2)))); err == nil {
		t.Fatal("ignored depth limit")
	}
	if err := yaml.Load([]byte("[&a 1, *a]"), &got, yaml.WithPlugin(p),
		yaml.WithPlugin(loaderlimits.New(loaderlimits.AliasValue(0)))); err == nil {
		t.Fatal("ignored alias limit")
	}
}

func TestMissingLibrary(t *testing.T) {
	t.Setenv("YAMLSTAR_LIBRARY_PATH", t.TempDir())
	if _, err := New(WithInstall(false)).Parse([]byte("true")); err == nil ||
		!strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("missing library: %v", err)
	}

}

func TestConcurrentLoads(t *testing.T) {
	useTestLibrary(t)
	p := New()
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 20 {
				var got map[string]int
				if err := yaml.Load([]byte("a: 42"), &got,
					yaml.WithPlugin(p)); err != nil || got["a"] != 42 {
					t.Errorf("got %v, %v", got, err)
				}
			}
		}()
	}
	wait.Wait()
}

func TestInvalidEventData(t *testing.T) {
	for _, input := range []string{
		"[", `[{:event "invalid"}]`,
		`[{:event "document_start" :version "1.3"}]`,
	} {
		if _, err := decodeEvents([]byte(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
