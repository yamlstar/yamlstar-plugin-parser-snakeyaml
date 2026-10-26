package goyaml

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	binary := os.Getenv("GO_YAML_CLI")
	if binary == "" {
		t.Skip("set GO_YAML_CLI to an adapter-enabled executable")
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", t.TempDir())
	for _, args := range [][]string{
		{"--help"}, {"--plugin=yaml-parser=snakeyaml", "--help"},
		{"-j"},
		{"--plugin=yaml-parser=go-yaml", "-j"},
	} {
		command := exec.Command(binary, args...)
		command.Stdin = strings.NewReader("answer: 42\n")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%v without a library: %v\n%s", args, err, output)
		}
		if args[len(args)-1] == "-j" && string(output) != "{\"answer\":42}\n" {
			t.Fatalf("native parsing: %s", output)
		}
	}
	useTestLibrary(t)
	for _, test := range []struct{ input, want string }{
		{"answer: 42\nitems: [one, two]\n", "{\"answer\":42,\"items\":[\"one\",\"two\"]}\n"},
		{"[unterminated", "while parsing a flow sequence"},
	} {
		command := exec.Command(binary, "--plugin=yaml-parser=snakeyaml", "-j")
		command.Stdin = strings.NewReader(test.input)
		output, err := command.CombinedOutput()
		if !strings.Contains(string(output), test.want) {
			t.Fatalf("got %s, %v; want %q", output, err, test.want)
		}
		if (err != nil) != (test.input == "[unterminated") {
			t.Fatalf("unexpected parse status: %v", err)
		}
	}
}

func TestSelectedCLI(t *testing.T) {
	binary := os.Getenv("GO_YAML_CLI")
	if binary == "" {
		t.Skip("set GO_YAML_CLI to an adapter-default CLI")
	}
	useTestLibrary(t)
	command := exec.Command(binary, "-j")
	command.Stdin = strings.NewReader("answer: 42")
	if output, err := command.CombinedOutput(); err != nil || string(output) != "{\"answer\":42}\n" {
		t.Fatalf("configured default: %v\n%s", err, output)
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", t.TempDir())
	command = exec.Command(binary, "--plugin=yaml-parser=go-yaml", "-j")
	command.Stdin = strings.NewReader("answer: 42")
	if output, err := command.CombinedOutput(); err != nil || string(output) != "{\"answer\":42}\n" {
		t.Fatalf("explicit native override: %v\n%s", err, output)
	}
}
