// file.go implements amifl-spec.md section 13.10's file I/O built-ins —
// step 12. FileHandle is the runtime representation behind AmiFL's opaque
// File type (section 2.2: "不透明なファイルハンドル、フィールド・メソッド
// 無し。生成・操作は13.10節の組み込み関数のみを通す") — AmiFL code only
// ever holds a *FileHandle, never its fields, matching every other opaque-
// type built-in this package backs.
package amiflrt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// FileHandle wraps an *os.File with a lazily-created *bufio.Reader — reading
// (readLine in particular) needs buffering that a bare *os.File doesn't
// provide, while writing goes straight to the underlying file with no
// buffering layer to flush.
type FileHandle struct {
	f *os.File
	r *bufio.Reader
}

// OpenFile implements `open(path, mode) -> Tuple2[File, Error]`. mode must
// be exactly "r", "w", or "a" (amifl-spec.md section 13.10's own wording) —
// any other value is a runtime error, since mode is an ordinary String
// argument (not a literal sema could restrict at compile time).
func OpenFile(path, mode string) (*FileHandle, error) {
	var flag int
	switch mode {
	case "r":
		flag = os.O_RDONLY
	case "w":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	default:
		return nil, fmt.Errorf("open: invalid mode %q (must be \"r\", \"w\", or \"a\")", mode)
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileHandle{f: f}, nil
}

func (fh *FileHandle) reader() *bufio.Reader {
	if fh.r == nil {
		fh.r = bufio.NewReader(fh.f)
	}
	return fh.r
}

// CloseFile implements `close(f) -> Error`.
func CloseFile(fh *FileHandle) error {
	return fh.f.Close()
}

// ReadFile implements `read(f, n) -> Tuple2[Bytes, Error]`: up to n bytes,
// fewer at EOF (io.Reader's own "some data, possibly with a non-nil err"
// contract — read is never required to fill the full n bytes it was asked
// for).
func ReadFile(fh *FileHandle, n int64) ([]byte, error) {
	buf := make([]byte, n)
	nRead, err := fh.reader().Read(buf)
	return buf[:nRead], err
}

// ReadAllFile implements `readAll(f) -> Tuple2[Bytes, Error]`.
func ReadAllFile(fh *FileHandle) ([]byte, error) {
	return io.ReadAll(fh.reader())
}

// ReadLineFile implements `readLine(f) -> Tuple2[String, Error]`, trimming
// the line's own trailing "\n" (and a preceding "\r", for CRLF files) —
// a line's terminator is a framing detail, not part of its value, the same
// way lines() (chan_files.go) strips it for every element of the Stream it
// produces. A final line with no trailing newline still reads successfully
// once (err == nil) — bufio.Reader.ReadString's own io.EOF alongside
// non-empty data — with the *next* call correctly reporting io.EOF against
// an empty read once the underlying reader is truly exhausted. codegen's
// files.go calls this in a hand-rolled loop to implement lines() -> Stream
// [String] the same way — this function itself never touches a channel.
func ReadLineFile(fh *FileHandle) (string, error) {
	line, err := fh.reader().ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && line == "" {
		return "", io.EOF
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

// WriteFile implements `write(f, data) -> Tuple2[Int, Error]`.
func WriteFile(fh *FileHandle, data []byte) (int64, error) {
	n, err := fh.f.Write(data)
	return int64(n), err
}

// Stdin/Stdout/Stderr implement `stdin`/`stdout`/`stderr` () -> File.
func Stdin() *FileHandle  { return &FileHandle{f: os.Stdin} }
func Stdout() *FileHandle { return &FileHandle{f: os.Stdout} }
func Stderr() *FileHandle { return &FileHandle{f: os.Stderr} }
