package goyaml

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestJVMNativeParity(t *testing.T) {
	classpath := os.Getenv("SNAKEYAML_JAVA_CLASSPATH")
	if classpath == "" {
		t.Skip("set SNAKEYAML_JAVA_CLASSPATH or run make test-parity")
	}
	useTestLibrary(t)
	lib, err := New().load()
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		"", "# comment", "plain", "[🙂, λ]\r\n", "|\n  last line",
		"a: &a [1, 'two', \"three\\0four\"]\nb: *a",
		"%YAML 1.2\n%TAG !e! tag:example.com,2026:\n--- !e!x value\n...\n",
		"---\na: 1\n---\nfalse\n",
	} {
		t.Run(input, func(t *testing.T) {
			command := exec.Command("java", "-cp", classpath,
				"org.yamlstar.snakeyaml.EventEdn")
			command.Stdin = bytes.NewBufferString(input)
			want, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("JVM: %v\n%s", err, want)
			}
			got, err := lib.parse([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("native: %s\nJVM: %s", got, want)
			}
		})
	}
}

func TestSourceMarksAndDirectives(t *testing.T) {
	useTestLibrary(t)
	events, err := New().Parse([]byte(
		"%TAG !e! tag:example.com,2026:\n--- !e!x [🙂, λ]"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events[1].TagDirectives) != 1 ||
		events[1].TagDirectives[0].Handle != "!e!" ||
		events[1].TagDirectives[0].Prefix != "tag:example.com,2026:" {
		t.Fatalf("lost directives: %+v", events[1])
	}
	if events[3].StartMark.Line != 2 || events[3].Value != "🙂" ||
		events[4].StartMark.Column != events[3].StartMark.Column+3 {
		t.Fatalf("lost code-point positions: %+v, %+v", events[3], events[4])
	}
	events, err = New().Parse([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if events[2].EndMark.Index != 5 || events[2].EndMark.Line != 1 ||
		events[2].EndMark.Column != 6 {
		t.Fatalf("rewritten input: %+v", events[2])
	}
}
