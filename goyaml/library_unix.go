//go:build darwin || freebsd || linux

package goyaml

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"olympos.io/encoding/edn"
)

var libraries = struct {
	sync.Mutex
	byPath map[string]*library
}{byPath: make(map[string]*library)}

func loadLibrary() (*library, error) {
	name := libraryFilename()
	directories := searchDirectories()
	for _, dir := range directories {
		if dir == "" {
			continue
		}
		path, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect snakeyaml library %s: %w", path, err)
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("snakeyaml library %s is not a regular file", path)
		}
		libraries.Lock()
		defer libraries.Unlock()
		if lib := libraries.byPath[path]; lib != nil {
			return lib, nil
		}
		lib, err := openLibrary(path)
		if err == nil {
			libraries.byPath[path] = lib
		}
		return lib, err
	}
	return nil, fmt.Errorf("%w: cannot find %s in %s; set YAMLSTAR_LIBRARY_PATH",
		errLibraryNotFound, name,
		strings.Join(directories, string(os.PathListSeparator)))
}

func openLibrary(path string) (lib *library, err error) {
	defer func() {
		if value := recover(); value != nil {
			lib, err = nil, fmt.Errorf("load snakeyaml library %s: %v", path, value)
		}
	}()
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load snakeyaml library %s: %w", path, err)
	}
	// Native Image libraries remain resident for the process lifetime.
	var abi func() uint64
	var manifest func(**byte, *uintptr) int32
	var transform func(*byte, uintptr, *byte, uintptr, **byte, *uintptr) int32
	var free func(*byte)
	purego.RegisterLibFunc(&abi, handle, "yamlstar_plugin_v2_abi")
	purego.RegisterLibFunc(&manifest, handle, "yamlstar_plugin_v2_manifest")
	purego.RegisterLibFunc(&transform, handle, "yamlstar_plugin_v2_transform")
	purego.RegisterLibFunc(&free, handle, "yamlstar_plugin_v2_free")
	if version := abi(); version != 2 {
		return nil, fmt.Errorf("snakeyaml library ABI %d, expected 2", version)
	}
	takeOutput := func(status int32, output *byte, length uintptr) ([]byte, error) {
		if output != nil {
			defer free(output)
		}
		if (output == nil && length != 0) || length > uintptr(^uint(0)>>1) {
			return nil, fmt.Errorf("snakeyaml library returned an invalid buffer")
		}
		data := append([]byte(nil), unsafe.Slice(output, int(length))...)
		if status != 0 {
			return nil, fmt.Errorf("snakeyaml status %d: %s", status, data)
		}
		return data, nil
	}
	var output *byte
	var length uintptr
	status := manifest(&output, &length)
	data, err := takeOutput(status, output, length)
	if err != nil {
		return nil, err
	}
	version, err := validateManifest(data)
	if err != nil {
		return nil, fmt.Errorf("snakeyaml library %s: %w", path, err)
	}
	return &library{version: version, parse: func(input []byte) ([]byte, error) {
		options := []byte("{}")
		var output *byte
		var length uintptr
		var inputPtr *byte
		if len(input) > 0 {
			inputPtr = &input[0]
		}
		status := transform(inputPtr, uintptr(len(input)), &options[0],
			uintptr(len(options)), &output, &length)
		runtime.KeepAlive(input)
		runtime.KeepAlive(options)
		return takeOutput(status, output, length)
	}}, nil
}

func validateManifest(data []byte) (string, error) {
	var info struct {
		ABI         int    `edn:"abi"`
		API         string `edn:"api"`
		Name        string `edn:"name"`
		Version     string `edn:"version"`
		Kind        string `edn:"kind"`
		EventFormat string `edn:"event-format"`
	}
	if err := edn.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("snakeyaml manifest: %w", err)
	}
	if info.ABI != 2 || info.API != "yaml-parser" ||
		info.Name != "snakeyaml" || info.Kind != "event-source" ||
		info.EventFormat != "yamlstar-events-edn-v1" ||
		info.Version == "" {
		return "", fmt.Errorf("incompatible snakeyaml manifest: %s", data)
	}
	if info.Version != Version {
		return "", fmt.Errorf("version mismatch: expected %s, found %s; "+
			"select a matching library with YAMLSTAR_LIBRARY_PATH",
			Version, info.Version)
	}
	return info.Version, nil
}
