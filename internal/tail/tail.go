// Package tail reads the last lines of a file without reading the whole file,
// so a log of many megabytes costs one or two blocks.
package tail

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

const block = 64 * 1024

// Last returns the last n lines of the file at path, whole lines only, joined
// by newlines with none at the end.
//
// @param path {string} the file to read
// @param n {int} how many lines from the end, at least 1
// @returns {string, error} the lines, or the error from opening or reading the file
// @example
//
//	text, err := tail.Last("/var/log/nginx/error.log", 10000)
func Last(path string, n int) (string, error) {
	if n < 1 {
		return "", fmt.Errorf("the number of lines must be at least 1 (got %d)", n)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", &fs.PathError{Op: "read", Path: path, Err: errors.New("is a directory")}
	}
	var buf []byte
	pos := info.Size()
	for pos > 0 && bytes.Count(buf, []byte("\n")) <= n {
		size := int64(block)
		if pos < size {
			size = pos
		}
		pos -= size
		chunk := make([]byte, size)
		if _, err := f.ReadAt(chunk, pos); err != nil && err != io.EOF {
			return "", err
		}
		buf = append(chunk, buf...)
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if pos > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line may have been cut by the block boundary
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return "", nil
	}
	return strings.Join(lines, "\n"), nil
}
