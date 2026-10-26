package goyaml

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ulikunitz/xz"
)

var (
	errLibraryNotFound = errors.New("snakeyaml library not found")
	installMu          sync.Mutex
)

const maxArchiveSize = 128 << 20

type installer struct {
	client  *http.Client
	baseURL string
}

func newInstaller() *installer {
	return &installer{
		client: &http.Client{Timeout: 60 * time.Second},
		baseURL: "https://github.com/yamlstar/" +
			"yamlstar-plugin-parser-snakeyaml/releases/download",
	}
}

func releasePlatform(system, arch string) (string, error) {
	switch system + "/" + arch {
	case "linux/amd64":
		return "linux-x64", nil
	case "linux/arm64":
		return "linux-aarch64", nil
	case "darwin/arm64":
		return "macos-arm64", nil
	default:
		return "", fmt.Errorf("snakeyaml automatic installation is unsupported "+
			"on %s/%s; install the library manually", system, arch)
	}
}

func (i *installer) install(directories []string) error {
	platform, err := releasePlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	directory, temporary, err := installDirectory(directories)
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)

	name := "yamlstar-plugin-parser-snakeyaml-" + Version + "-" + platform
	archiveName := name + ".tar.xz"
	base := i.baseURL + "/" + Version + "/"
	var checksums bytes.Buffer
	if err := i.download(base+"SHA256SUMS", &checksums, 1<<20); err != nil {
		return err
	}
	expected, err := archiveChecksum(checksums.Bytes(), archiveName)
	if err != nil {
		return err
	}
	archive, err := os.Create(filepath.Join(temporary, archiveName))
	if err != nil {
		return err
	}
	defer archive.Close()
	hash := sha256.New()
	if err := i.download(base+archiveName,
		io.MultiWriter(archive, hash), maxArchiveSize); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("snakeyaml checksum verification failed for %s", archiveName)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged := filepath.Join(temporary, libraryFilename())
	if err := extractLibrary(archive, name, staged); err != nil {
		return err
	}
	// Validate the staged library before making it visible to other processes.
	if _, err := openLibrary(staged); err != nil {
		return err
	}
	destination := filepath.Join(directory, libraryFilename())
	// A hard link publishes the complete file without replacing a concurrent
	// install or a library placed here by another application.
	if err := os.Link(staged, destination); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("install snakeyaml at %s: %w", destination, err)
		}
		_, err = openLibrary(destination)
		return err
	}
	fmt.Fprintf(os.Stderr, "Installed SnakeYAML %s at %s\n", Version, destination)
	return nil
}

func installDirectory(directories []string) (string, string, error) {
	for _, directory := range directories {
		if directory == "" {
			continue
		}
		if err := os.MkdirAll(directory, 0755); err != nil {
			continue
		}
		temporary, err := os.MkdirTemp(directory, ".snakeyaml-install-*")
		if err == nil {
			return directory, temporary, nil
		}
	}
	return "", "", fmt.Errorf("no writable SnakeYAML plugin directory in %s; "+
		"set YAMLSTAR_LIBRARY_PATH", strings.Join(directories, string(os.PathListSeparator)))
}

func (i *installer) download(url string, output io.Writer, limit int64) error {
	response, err := i.client.Get(url)
	if err != nil {
		return fmt.Errorf("download snakeyaml %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download snakeyaml %s: HTTP %s", url, response.Status)
	}
	n, err := io.Copy(output, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return fmt.Errorf("download snakeyaml %s: %w", url, err)
	}
	if n > limit {
		return fmt.Errorf("snakeyaml download exceeds size limit: %s", url)
	}
	return nil
}

func archiveChecksum(data []byte, name string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	checksum := ""
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.TrimPrefix(
			strings.TrimPrefix(fields[1], "*"), "./") != name {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size || checksum != "" {
			return "", fmt.Errorf("invalid SHA256SUMS entry for %s", name)
		}
		checksum = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("invalid SHA256SUMS: %w", err)
	}
	if checksum == "" {
		return "", fmt.Errorf("SHA256SUMS has no checksum for %s", name)
	}
	return checksum, nil
}

func extractLibrary(archive io.Reader, release, destination string) error {
	uncompressed, err := xz.NewReader(archive)
	if err != nil {
		return fmt.Errorf("snakeyaml archive: %w", err)
	}
	limited := &io.LimitedReader{R: uncompressed, N: 4*maxArchiveSize + 1}
	reader := tar.NewReader(limited)
	found := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("snakeyaml archive: %w", err)
		}
		if path.IsAbs(header.Name) || strings.Contains(header.Name, "\\") {
			return fmt.Errorf("unsafe snakeyaml archive path %q", header.Name)
		}
		for _, part := range strings.Split(header.Name, "/") {
			if part == ".." {
				return fmt.Errorf("unsafe snakeyaml archive path %q", header.Name)
			}
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("snakeyaml archive contains a link or special file: %s",
				header.Name)
		}
		name := path.Clean(header.Name)
		if name != release+"/lib/"+libraryFilename() {
			continue
		}
		if found || header.Typeflag != tar.TypeReg || header.Size > maxArchiveSize {
			return errors.New("invalid snakeyaml archive library entry")
		}
		found = true
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return fmt.Errorf("extract snakeyaml library: %w", err)
		}
	}
	// Consume the XZ trailer too, so corrupt or truncated streams are rejected.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return fmt.Errorf("snakeyaml archive: %w", err)
	}
	if limited.N == 0 {
		return errors.New("snakeyaml archive exceeds size limit")
	}
	if !found {
		return fmt.Errorf("snakeyaml archive does not contain %s", libraryFilename())
	}
	return nil
}
