package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adam/tau/internal/types"
)

// JSONLWriter provides append-only writes to a JSONL session file.
type JSONLWriter struct {
	f   *os.File
	buf *bufio.Writer
}

// NewJSONLWriter creates a new append-only writer for the given file path.
// The parent directory must already exist.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	return &JSONLWriter{
		f:   f,
		buf: bufio.NewWriter(f),
	}, nil
}

// WriteHeader writes the session header as the first line of the JSONL file.
func (w *JSONLWriter) WriteHeader(h types.SessionHeader) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal session header: %w", err)
	}
	if _, err := w.buf.Write(data); err != nil {
		return err
	}
	if err := w.buf.WriteByte('\n'); err != nil {
		return err
	}
	return w.buf.Flush()
}

// WriteEntry appends a session entry as a single JSON line.
func (w *JSONLWriter) WriteEntry(e types.SessionEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal session entry: %w", err)
	}
	if _, err := w.buf.Write(data); err != nil {
		return err
	}
	if err := w.buf.WriteByte('\n'); err != nil {
		return err
	}
	return w.buf.Flush()
}

// Sync flushes the buffer and syncs the file to disk.
func (w *JSONLWriter) Sync() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}
	return w.f.Sync()
}

// Close flushes any remaining data and closes the underlying file.
func (w *JSONLWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}
	return w.f.Close()
}

// ReadEntries opens a session file, parses the header, and returns all valid entries.
// If the last line is incomplete (corruption), it is silently discarded and the
// valid entries up to that point are returned.
func ReadEntries(path string) (*types.SessionHeader, []types.SessionEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	var header *types.SessionHeader
	var entries []types.SessionEntry
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // skip empty lines
		}
		lineNum++

		if lineNum == 1 {
			// First line must be the session header
			var h types.SessionHeader
			if err := json.Unmarshal(line, &h); err != nil {
				return nil, nil, fmt.Errorf("parse session header (line 1): %w", err)
			}
			if h.Type != "session" {
				return nil, nil, fmt.Errorf("invalid header type: %q, expected 'session'", h.Type)
			}
			if h.Version < 1 {
				return nil, nil, fmt.Errorf("unsupported session version: %d", h.Version)
			}
			header = &h
			continue
		}

		// Parse entry
		var entry types.SessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Corruption: stop reading, return what we have
			// The incomplete last line is discarded
			break
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading session file: %w", err)
	}

	if header == nil {
		return nil, nil, fmt.Errorf("empty session file: no header found")
	}

	return header, entries, nil
}

// CreateSessionFile creates a new session file with the given header in dirPath.
// Returns the full file path.
func CreateSessionFile(dirPath string, header types.SessionHeader) (string, *JSONLWriter, error) {
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", nil, fmt.Errorf("create session directory: %w", err)
	}

	filename := GenerateFilename(header.Timestamp, header.ID)
	fullPath := filepath.Join(dirPath, filename)

	writer, err := NewJSONLWriter(fullPath)
	if err != nil {
		return "", nil, err
	}

	if err := writer.WriteHeader(header); err != nil {
		writer.Close()
		return "", nil, fmt.Errorf("write session header: %w", err)
	}

	return fullPath, writer, nil
}

// now returns the current time. Package-level variable for test override.
var now = time.Now

// GenerateID creates a unique 8-character hex ID.
func GenerateID() string {
	return newID()
}

// newID generates a unique 8-character hex ID.
func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
