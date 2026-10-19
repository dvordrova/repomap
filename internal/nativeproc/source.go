// Package nativeproc holds what the native helper processes share.
package nativeproc

import (
	"fmt"
	"os"
)

// SourceFile writes an embedded interpreter source to a temporary file so the
// interpreter reads it from disk instead of argv: Linux caps one argv string
// at 128 KiB (MAX_ARG_STRLEN), and the JS and Python helpers passed with
// --eval and -c had grown past it, so neither started there. The file lives
// for one invocation; remove releases it and may be called more than once.
func SourceFile(pattern, source string) (path string, remove func(), err error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("native helper source: %w", err)
	}
	path = file.Name()
	remove = func() { os.Remove(path) }
	if _, err := file.WriteString(source); err != nil {
		file.Close()
		remove()
		return "", nil, fmt.Errorf("native helper source: %w", err)
	}
	if err := file.Close(); err != nil {
		remove()
		return "", nil, fmt.Errorf("native helper source: %w", err)
	}
	return path, remove, nil
}
