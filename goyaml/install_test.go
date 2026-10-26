//go:build darwin || freebsd || linux

package goyaml

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ulikunitz/xz"
)

type archiveEntry struct {
	name string
	data []byte
	kind byte
}

func testArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	compressed, err := xz.NewWriter(&output)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(compressed)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: 0755,
			Size: int64(len(entry.data)), Typeflag: kind}
		if kind == tar.TypeSymlink || kind == tar.TypeLink {
			header.Linkname = "target"
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func testRelease(t *testing.T) string {
	t.Helper()
	platform, err := releasePlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	return "yamlstar-plugin-parser-snakeyaml-" + Version + "-" + platform
}

func releaseFixture(t *testing.T, archive []byte, checksum string,
	status int) (*installer, *atomic.Int32) {
	t.Helper()
	name := testRelease(t) + ".tar.xz"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		switch r.URL.Path {
		case "/" + Version + "/SHA256SUMS":
			fmt.Fprintf(w, "%s  ./%s\n", checksum, name)
		case "/" + Version + "/" + name:
			_, _ = w.Write(archive)
		default:
			t.Errorf("unexpected download %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return &installer{client: server.Client(), baseURL: server.URL}, &requests
}

func TestInstallAndReuse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "lib", libraryFilename()))
	if err != nil {
		t.Fatal(err)
	}
	release := testRelease(t)
	archive := testArchive(t, archiveEntry{
		name: release + "/lib/" + libraryFilename(), data: data})
	download, requests := releaseFixture(t, archive,
		fmt.Sprintf("%x", sha256.Sum256(archive)), http.StatusOK)
	directory := t.TempDir()
	t.Setenv("YAMLSTAR_LIBRARY_PATH", directory)
	offline := New(WithInstall(false), func(p *Plugin) { p.installer = download })
	if _, err := offline.Parse([]byte("true")); err == nil || requests.Load() != 0 {
		t.Fatalf("disabled installation: %v, %d requests", err, requests.Load())
	}
	var wait sync.WaitGroup
	// Separate parser instances must coordinate installation too.
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			parser := New(func(p *Plugin) { p.installer = download })
			if _, err := parser.Parse([]byte("answer: 42")); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	if requests.Load() != 2 {
		t.Fatalf("got %d requests; want one checksum and one archive", requests.Load())
	}
	installed, err := os.ReadFile(filepath.Join(directory, libraryFilename()))
	if err != nil || !bytes.Equal(installed, data) {
		t.Fatalf("installed library differs: %v", err)
	}
	assertNoStagingFiles(t, directory)
	if _, err := New(WithInstall(false)).Parse([]byte("true")); err != nil {
		t.Fatalf("reuse installed library offline: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatal("reuse downloaded the library again")
	}
}

func assertNoStagingFiles(t *testing.T, directory string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, ".snakeyaml-install-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("staging files remain: %v, %v", files, err)
	}
}

func TestInstallFailures(t *testing.T) {
	name := testRelease(t) + "/lib/" + libraryFilename()
	invalidLibrary := testArchive(t, archiveEntry{name: name, data: []byte("invalid library")})
	for _, test := range []struct {
		name, want string
		archive    []byte
		checksum   string
		status     int
	}{
		{name: "unavailable", status: http.StatusNotFound, want: "HTTP 404"},
		{name: "checksum", archive: invalidLibrary,
			checksum: strings.Repeat("0", 64), want: "checksum verification failed"},
		{name: "invalid checksum", archive: invalidLibrary,
			checksum: "invalid", want: "invalid SHA256SUMS"},
		{name: "invalid archive", archive: []byte("invalid"), want: "archive"},
		{name: "invalid library", archive: invalidLibrary, want: "load snakeyaml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := test.status
			if status == 0 {
				status = http.StatusOK
			}
			checksum := test.checksum
			if checksum == "" {
				checksum = fmt.Sprintf("%x", sha256.Sum256(test.archive))
			}
			download, _ := releaseFixture(t, test.archive, checksum, status)
			directory := t.TempDir()
			err := download.install([]string{directory})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v; want %s", err, test.want)
			}
			if _, err := os.Stat(filepath.Join(directory, libraryFilename())); !os.IsNotExist(err) {
				t.Fatalf("failed install published a library: %v", err)
			}
			assertNoStagingFiles(t, directory)
		})
	}
}

func TestExistingLibraryIsPreserved(t *testing.T) {
	download, requests := releaseFixture(t, nil, "", http.StatusOK)
	for _, data := range []string{"not a library", ""} {
		directory := t.TempDir()
		name := filepath.Join(directory, libraryFilename())
		if err := os.WriteFile(name, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("YAMLSTAR_LIBRARY_PATH", directory)
		parser := New(func(p *Plugin) { p.installer = download })
		if _, err := parser.Parse([]byte("true")); err == nil {
			t.Fatal("accepted incompatible existing library")
		}
		got, err := os.ReadFile(name)
		if err != nil || string(got) != data {
			t.Fatalf("existing library changed: %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("incompatible existing library triggered a download")
	}
}

func TestMismatchedInstalledVersion(t *testing.T) {
	manifest, err := os.ReadFile("../plugin.edn")
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.ReplaceAll(string(manifest), Version, "0.0.1")
	directory := t.TempDir()
	name := filepath.Join(directory, libraryFilename())
	source := fmt.Sprintf(`
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
uint64_t yamlstar_plugin_v2_abi(void) { return 2; }
int32_t yamlstar_plugin_v2_manifest(char **out, uintptr_t *length) {
    const char *data = %q;
    *length = strlen(data);
    *out = malloc(*length);
    memcpy(*out, data, *length);
    return 0;
}
int32_t yamlstar_plugin_v2_transform(void *in, uintptr_t n,
    void *opts, uintptr_t m, char **out, uintptr_t *length) {
    *out = NULL;
    *length = 0;
    return 1;
}
void yamlstar_plugin_v2_free(void *data) { free(data); }
`, wrong)
	sourceFile := filepath.Join(t.TempDir(), "fixture.c")
	if err := os.WriteFile(sourceFile, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	flags := []string{"-shared", "-fPIC"}
	if runtime.GOOS == "darwin" {
		flags = []string{"-dynamiclib"}
	}
	flags = append(flags, "-o", name, sourceFile)
	if output, err := exec.Command("cc", flags...).CombinedOutput(); err != nil {
		t.Fatalf("build mismatched ABI fixture: %v\n%s", err, output)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", directory)
	download, requests := releaseFixture(t, nil, "", http.StatusOK)
	parser := New(func(p *Plugin) { p.installer = download })
	_, err = parser.Parse([]byte("true"))
	for _, want := range []string{"version mismatch", Version, "0.0.1", name,
		"YAMLSTAR_LIBRARY_PATH"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in version mismatch: %v", want, err)
		}
	}
	got, readErr := os.ReadFile(name)
	if readErr != nil || !bytes.Equal(got, data) || requests.Load() != 0 {
		t.Fatalf("version conflict changed installation: %v, %d requests",
			readErr, requests.Load())
	}
}

func TestConcurrentInstallProcesses(t *testing.T) {
	if url := os.Getenv("RAPIDYAML_INSTALL_TEST_URL"); url != "" {
		parser := New(func(p *Plugin) {
			p.installer = &installer{client: http.DefaultClient, baseURL: url}
		})
		if _, err := parser.Parse([]byte("true")); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(filepath.Join("..", "lib", libraryFilename()))
	if err != nil {
		t.Fatal(err)
	}
	archive := testArchive(t, archiveEntry{
		name: testRelease(t) + "/lib/" + libraryFilename(), data: data})
	download, requests := releaseFixture(t, archive,
		fmt.Sprintf("%x", sha256.Sum256(archive)), http.StatusOK)
	directory := t.TempDir()
	t.Setenv("YAMLSTAR_LIBRARY_PATH", directory)
	t.Setenv("RAPIDYAML_INSTALL_TEST_URL", download.baseURL)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestConcurrentInstallProcesses$")
			if output, err := command.CombinedOutput(); err != nil {
				t.Errorf("concurrent installer: %v\n%s", err, output)
			}
		}()
	}
	wait.Wait()
	if requests.Load() < 2 || requests.Load() > 4 {
		t.Fatalf("unexpected request count: %d", requests.Load())
	}
	installed, err := os.ReadFile(filepath.Join(directory, libraryFilename()))
	if err != nil || !bytes.Equal(installed, data) {
		t.Fatalf("concurrent installation changed library: %v", err)
	}
	assertNoStagingFiles(t, directory)
}

func TestArchiveValidation(t *testing.T) {
	release := testRelease(t)
	library := archiveEntry{name: release + "/lib/" + libraryFilename(), data: []byte("library")}
	for _, test := range []struct {
		name    string
		entries []archiveEntry
	}{
		{"absolute", []archiveEntry{{name: "/escape"}, library}},
		{"parent", []archiveEntry{{name: release + "/../escape"}, library}},
		{"backslash", []archiveEntry{{name: `..\escape`}, library}},
		{"symlink", []archiveEntry{library, {name: release + "/link", kind: tar.TypeSymlink}}},
		{"hardlink", []archiveEntry{library, {name: release + "/link", kind: tar.TypeLink}}},
		{"special", []archiveEntry{library, {name: release + "/fifo", kind: tar.TypeFifo}}},
		{"missing", []archiveEntry{{name: release + "/License"}}},
		{"duplicate", []archiveEntry{library, library}},
	} {
		t.Run(test.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "library")
			if err := extractLibrary(bytes.NewReader(testArchive(t, test.entries...)),
				release, destination); err == nil {
				t.Fatal("accepted unsafe or incomplete archive")
			}
		})
	}
	archive := testArchive(t, library)
	if err := extractLibrary(bytes.NewReader(archive[:len(archive)-8]), release,
		filepath.Join(t.TempDir(), "library")); err == nil {
		t.Fatal("accepted truncated XZ stream")
	}
}

func TestInstallDirectory(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installDirectory([]string{"", blocked}); err == nil {
		t.Fatal("accepted an unwritable destination")
	}
	writable := t.TempDir()
	directory, temporary, err := installDirectory([]string{blocked, writable})
	if err != nil || directory != writable {
		t.Fatalf("did not select first writable directory: %s, %v", directory, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporary) })
}

func TestReleasePlatforms(t *testing.T) {
	for input, want := range map[string]string{
		"linux/amd64": "linux-x64", "linux/arm64": "linux-aarch64",
		"darwin/arm64": "macos-arm64",
	} {
		parts := strings.Split(input, "/")
		if got, err := releasePlatform(parts[0], parts[1]); err != nil || got != want {
			t.Fatalf("%s: got %s, %v; want %s", input, got, err, want)
		}
	}
	if _, err := releasePlatform("darwin", "amd64"); err == nil {
		t.Fatal("accepted macOS Intel native installation")
	}
	if _, err := releasePlatform("windows", "amd64"); err == nil {
		t.Fatal("accepted unsupported release platform")
	}
}

func TestManifestVersion(t *testing.T) {
	data, err := os.ReadFile("../plugin.edn")
	if err != nil {
		t.Fatal(err)
	}
	if version, err := validateManifest(data); err != nil || version != Version {
		t.Fatalf("version metadata differs: %s, %v", version, err)
	}
	for _, pair := range [][2]string{
		{Version, "0.0.1"}, {":abi 2", ":abi 1"},
		{`"yaml-parser"`, `"parser"`}, {`"snakeyaml"`, `"other"`},
	} {
		changed := bytes.ReplaceAll(data, []byte(pair[0]), []byte(pair[1]))
		if _, err := validateManifest(changed); err == nil {
			t.Fatalf("accepted incompatible manifest %s", changed)
		}
	}
}

func TestSearchPathOverride(t *testing.T) {
	want := []string{"first", "second"}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", strings.Join(want, string(os.PathListSeparator)))
	if got := searchDirectories(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	t.Setenv("YAMLSTAR_LIBRARY_PATH", "")
	if got := searchDirectories(); len(got) != 0 {
		t.Fatalf("empty override used default directories: %v", got)
	}
}
