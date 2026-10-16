package engine

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type LogChunk struct {
	Text     string `json:"text"`
	Disabled bool   `json:"disabled,omitempty"`
	// Data preserves bytes even when cursor/limit splits UTF-8 or output is binary.
	Data       []byte `json:"data"`
	Cursor     int64  `json:"cursor"`
	NextCursor int64  `json:"nextCursor"`
	Size       int64  `json:"size"`
	EOF        bool   `json:"eof"`
	Truncated  bool   `json:"truncated"`
}

// Keep real handles for independent children, including when output is discarded.
func openOutput(path string, omit bool, flags int) (*os.File, error) {
	if omit {
		return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	return os.OpenFile(path, flags, 0600)
}

func (c *Client) readLog(query url.Values) (LogChunk, error) {
	chunk := LogChunk{Data: []byte{}}
	id := query.Get("id")
	if !validID.MatchString(id) {
		return chunk, errors.New("invalidRequest")
	}
	limit := int64(128 << 10)
	if query.Has("limit") {
		n, err := strconv.ParseInt(query.Get("limit"), 10, 64)
		if err != nil || n < 1 || n > 1<<20 {
			return chunk, errors.New("invalidLogLimit")
		}
		limit = n
	}
	if query.Has("cursor") {
		n, err := strconv.ParseInt(query.Get("cursor"), 10, 64)
		if err != nil || n < 0 {
			return chunk, errors.New("invalidLogCursor")
		}
		chunk.Cursor = n
	}
	var state executionState
	if err := readJSON(c.runPath(id, "state.json"), &state); err == nil {
		if state.Schema != ExecutionSchema || state.Run.ID != id {
			return chunk, errors.New("executionSchemaUnsupported")
		}
		if state.Run.OmitOutput {
			chunk.Cursor, chunk.NextCursor = 0, 0
			chunk.Disabled, chunk.EOF = true, true
			return chunk, nil
		}
	} else if !os.IsNotExist(err) {
		return chunk, err
	}
	file, err := os.Open(filepath.Join(c.dir, "logs", id+".log"))
	if os.IsNotExist(err) {
		if _, err = c.Run(id); err == nil {
			chunk.EOF = true
			return chunk, nil
		}
	}
	if err != nil {
		return chunk, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return chunk, err
	}
	chunk.Size = info.Size()
	if !query.Has("cursor") {
		chunk.Cursor = max(0, chunk.Size-limit)
	}
	if chunk.Cursor > chunk.Size {
		return chunk, errors.New("logCursorBeyondEnd")
	}
	chunk.Data = make([]byte, min(limit, chunk.Size-chunk.Cursor))
	n, err := file.ReadAt(chunk.Data, chunk.Cursor)
	if err != nil && err != io.EOF {
		return chunk, err
	}
	chunk.Data = chunk.Data[:n]
	chunk.Text = string(chunk.Data)
	chunk.NextCursor = chunk.Cursor + int64(n)
	chunk.EOF = chunk.NextCursor >= chunk.Size
	chunk.Truncated = chunk.Cursor > 0 || !chunk.EOF
	return chunk, nil
}
