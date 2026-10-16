package main

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Raw bytes preserve UTF-8 split between events. Only the bounded display tail is
// retained; switching runs discards the buffer and the previous subscription.
type runOutputViewModel struct {
	output, logIssue string
	outputTruncated  bool
	bytes            []byte
	cursor           int64
	trimmed          bool
}

func (v *runOutputViewModel) accept(event logEvent) error {
	chunk := event.Chunk
	if chunk == nil || chunk.Cursor < 0 || chunk.NextCursor != chunk.Cursor+int64(len(chunk.Data)) || chunk.Size < chunk.NextCursor {
		return errors.New("connectionLost")
	}
	if !event.Reset && chunk.Cursor != v.cursor {
		return errors.New("connectionLost")
	}
	if event.Reset {
		v.bytes = nil
		v.trimmed = chunk.Cursor > 0
	}
	v.bytes = append(v.bytes, chunk.Data...)
	const limit = 128 << 10
	if len(v.bytes) > limit {
		start := len(v.bytes) - limit
		for start < len(v.bytes) && !utf8.RuneStart(v.bytes[start]) {
			start++
		}
		v.bytes = v.bytes[:copy(v.bytes, v.bytes[start:])]
		v.trimmed = true
	}
	v.cursor = chunk.NextCursor
	v.output = strings.ToValidUTF8(string(v.bytes), "\uFFFD")
	v.outputTruncated = v.trimmed || !chunk.EOF
	v.logIssue = ""
	return nil
}
