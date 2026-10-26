//go:build !darwin && !freebsd && !linux

package goyaml

import (
	"fmt"
	"runtime"
)

func loadLibrary() (*library, error) {
	return nil, fmt.Errorf("snakeyaml shared libraries are unsupported on %s",
		runtime.GOOS)
}

func openLibrary(_ string) (*library, error) { return loadLibrary() }
