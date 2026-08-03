package workspace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/klauspost/compress/zstd"
)

// An archive is a zstd-compressed stream of newline-delimited JSON:
//
//	line 1  manifest
//	line n  {"t":"<table>","r":{...}}   in orderedTables order
//	last    {"t":"$summary","r":{...}}
//
// A flat record stream is used rather than tar or zip because both of those
// want an entry size up front, which would force buffering a table in memory
// or staging it on disk. Workspaces reach tens of gigabytes, so neither is
// acceptable. Writing records in dependency order lets the importer consume the
// stream in a single sequential pass.
const summaryTag = "$summary"

// WorkspaceInfo identifies the workspace an archive was taken from.
type WorkspaceInfo struct {
	ID          uint   `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ExcludedTable records a table that was deliberately not made importable.
type ExcludedTable struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
}

// Manifest is the first line of an archive.
type Manifest struct {
	FormatVersion int           `json:"format_version"`
	CreatedAt     time.Time     `json:"created_at"`
	Workspace     WorkspaceInfo `json:"workspace"`
	// IdentifierBases holds, per table, the lowest bigint row identifier in the
	// archive. Import shifts each table relative to its own base, so the
	// identifier space grows by the archive's span rather than by the size of
	// the target database. Without it, importing into the same database
	// repeatedly doubles the highest identifier each time and exhausts the
	// bigint range within a few dozen imports.
	IdentifierBases map[string]int64 `json:"identifier_bases,omitempty"`
	Excluded        []ExcludedTable  `json:"excluded_tables,omitempty"`
}

// Summary is the last line of an archive. Import compares it against what it
// actually read, which is how a truncated archive is detected.
type Summary struct {
	RowsByTable map[string]int64 `json:"rows_by_table"`
	TotalRows   int64            `json:"total_rows"`
}

type record struct {
	Table string          `json:"t"`
	Row   json.RawMessage `json:"r"`
}

type archiveWriter struct {
	zstd *zstd.Encoder
	buf  *bufio.Writer
}

func newArchiveWriter(w io.Writer, manifest Manifest) (*archiveWriter, error) {
	encoder, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("creating zstd encoder: %w", err)
	}
	aw := &archiveWriter{zstd: encoder, buf: bufio.NewWriterSize(encoder, 1<<20)}

	if err := aw.writeJSON(manifest); err != nil {
		encoder.Close()
		return nil, err
	}
	return aw, nil
}

func (a *archiveWriter) writeJSON(v any) error {
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding archive line: %w", err)
	}
	if _, err := a.buf.Write(encoded); err != nil {
		return fmt.Errorf("writing archive line: %w", err)
	}
	return a.buf.WriteByte('\n')
}

// writeRow emits a pre-serialised row. row is written verbatim, so the caller
// controls exactly what lands on disk.
func (a *archiveWriter) writeRow(table string, row []byte) error {
	if _, err := a.buf.WriteString(`{"t":"`); err != nil {
		return err
	}
	if _, err := a.buf.WriteString(table); err != nil {
		return err
	}
	if _, err := a.buf.WriteString(`","r":`); err != nil {
		return err
	}
	if _, err := a.buf.Write(row); err != nil {
		return err
	}
	if _, err := a.buf.WriteString("}\n"); err != nil {
		return err
	}
	return nil
}

func (a *archiveWriter) close(summary Summary) error {
	if err := a.writeJSON(record{Table: summaryTag, Row: mustJSON(summary)}); err != nil {
		a.zstd.Close()
		return err
	}
	if err := a.buf.Flush(); err != nil {
		a.zstd.Close()
		return fmt.Errorf("flushing archive: %w", err)
	}
	if err := a.zstd.Close(); err != nil {
		return fmt.Errorf("closing zstd stream: %w", err)
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	encoded, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

type archiveReader struct {
	zstd     *zstd.Decoder
	scanner  *bufio.Scanner
	Manifest Manifest
}

// maxArchiveLine bounds a single row. Sukyan stores whole HTTP exchanges, and
// the largest history rows run to several megabytes once hex-encoded.
const maxArchiveLine = 256 << 20

func newArchiveReader(r io.Reader) (*archiveReader, error) {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("creating zstd decoder: %w", err)
	}

	scanner := bufio.NewScanner(decoder)
	scanner.Buffer(make([]byte, 0, 1<<20), maxArchiveLine)

	ar := &archiveReader{zstd: decoder, scanner: scanner}
	if !scanner.Scan() {
		decoder.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading manifest: %w", err)
		}
		return nil, fmt.Errorf("archive is empty")
	}
	if err := json.Unmarshal(scanner.Bytes(), &ar.Manifest); err != nil {
		decoder.Close()
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}
	if ar.Manifest.FormatVersion != ArchiveFormatVersion {
		decoder.Close()
		return nil, fmt.Errorf("unsupported archive format version %d (this build reads version %d)",
			ar.Manifest.FormatVersion, ArchiveFormatVersion)
	}
	return ar, nil
}

// next returns the following record. It returns io.EOF once the summary line has
// been consumed, with the summary available from the returned value.
func (a *archiveReader) next() (*record, error) {
	if !a.scanner.Scan() {
		if err := a.scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		return nil, io.ErrUnexpectedEOF
	}
	var rec record
	if err := json.Unmarshal(a.scanner.Bytes(), &rec); err != nil {
		return nil, fmt.Errorf("decoding archive record: %w", err)
	}
	return &rec, nil
}

func (a *archiveReader) close() {
	a.zstd.Close()
}
