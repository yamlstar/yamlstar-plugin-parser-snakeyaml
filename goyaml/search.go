package goyaml

import (
	"os"
	"path/filepath"
	"runtime"
)

func libraryFilename() string {
	if runtime.GOOS == "darwin" {
		return "libyamlstar-plugin-parser-snakeyaml.dylib"
	}
	return "libyamlstar-plugin-parser-snakeyaml.so"
}

func searchDirectories() []string {
	if configured, present := os.LookupEnv("YAMLSTAR_LIBRARY_PATH"); present {
		return filepath.SplitList(configured)
	}
	var dirs []string
	if executable, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(executable),
			filepath.Join(filepath.Dir(executable), "..", "lib"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "lib"))
	}
	return append(dirs, "/usr/local/lib", "/usr/lib")
}
