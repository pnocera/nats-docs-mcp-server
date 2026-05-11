package fetcher

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
)

// ArchiveLimits caps untrusted archive growth before contents are read.
type ArchiveLimits struct {
	MaxFiles            int
	MaxUncompressedSize int64
	MaxFileSize         int64
}

// DefaultArchiveLimits returns conservative limits for the NATS docs archive.
func DefaultArchiveLimits() ArchiveLimits {
	return ArchiveLimits{
		MaxFiles:            5000,
		MaxUncompressedSize: 256 << 20,
		MaxFileSize:         8 << 20,
	}
}

// ArchiveEntry is a single file inside an archive, keyed relative to the root.
type ArchiveEntry struct {
	Path    string
	Content []byte
}

// Archive is an in-memory snapshot of a validated zip archive.
type Archive struct {
	Root    string
	Entries map[string]ArchiveEntry
}

// OpenLocalArchive opens a local zip archive and validates entries before
// returning them in memory.
func OpenLocalArchive(ctx context.Context, archivePath string, limits ArchiveLimits) (*Archive, error) {
	if archivePath == "" {
		return nil, fmt.Errorf("archive path cannot be empty")
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive %q: %w", archivePath, err)
	}
	defer reader.Close()

	return openZip(ctx, &reader.Reader, limits)
}

// OpenArchiveBytes opens a zip archive from memory and validates entries before
// returning them in memory.
func OpenArchiveBytes(ctx context.Context, data []byte, limits ArchiveLimits) (*Archive, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("archive bytes cannot be empty")
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open archive bytes: %w", err)
	}
	return openZip(ctx, reader, limits)
}

// openZip validates and reads regular files from a zip reader.
func openZip(ctx context.Context, r *zip.Reader, limits ArchiveLimits) (*Archive, error) {
	limits = normalizeArchiveLimits(limits)

	archive := &Archive{
		Entries: make(map[string]ArchiveEntry),
	}

	var fileCount int
	var totalSize int64
	var root string

	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		name, err := normalizeArchiveName(f.Name)
		if err != nil {
			return nil, err
		}

		info := f.FileInfo()
		if info.IsDir() {
			continue
		}
		if info.Mode()&fs.ModeType != 0 {
			continue
		}

		clean := path.Clean(name)
		parts := strings.Split(clean, "/")
		if len(parts) < 2 || parts[0] == "." || parts[0] == "" {
			return nil, fmt.Errorf("archive entry %q is not under a single root directory", f.Name)
		}
		if root == "" {
			root = parts[0]
			archive.Root = root
		} else if parts[0] != root {
			return nil, fmt.Errorf("archive contains multiple root directories: %q and %q", root, parts[0])
		}

		rel := strings.Join(parts[1:], "/")
		if rel == "" || rel == "." {
			continue
		}

		if fileCount+1 > limits.MaxFiles {
			return nil, fmt.Errorf("archive file count exceeds limit of %d", limits.MaxFiles)
		}
		fileSize, err := zipFileSize(f)
		if err != nil {
			return nil, err
		}
		if fileSize > limits.MaxFileSize {
			return nil, fmt.Errorf("archive entry %q size %d exceeds per-file limit %d", rel, fileSize, limits.MaxFileSize)
		}
		if fileSize > limits.MaxUncompressedSize {
			return nil, fmt.Errorf("archive entry %q size %d exceeds total uncompressed limit %d", rel, fileSize, limits.MaxUncompressedSize)
		}
		if totalSize > limits.MaxUncompressedSize-fileSize {
			return nil, fmt.Errorf("archive uncompressed size exceeds limit of %d", limits.MaxUncompressedSize)
		}

		content, err := readZipFile(f, limits.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("failed to read archive entry %q: %w", rel, err)
		}
		if int64(len(content)) != fileSize {
			return nil, fmt.Errorf("archive entry %q size mismatch: header=%d read=%d", rel, fileSize, len(content))
		}

		fileCount++
		totalSize += fileSize
		archive.Entries[rel] = ArchiveEntry{Path: rel, Content: content}
	}

	if root == "" {
		return nil, fmt.Errorf("archive contains no regular files")
	}

	return archive, nil
}

// Get returns a single archive entry by relative path.
func (a *Archive) Get(p string) (ArchiveEntry, bool) {
	if a == nil {
		return ArchiveEntry{}, false
	}
	p = strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, "\\", "/")), "./")
	entry, ok := a.Entries[p]
	return entry, ok
}

// List returns archive paths matching pred in sorted order.
func (a *Archive) List(pred func(string) bool) []string {
	if a == nil {
		return nil
	}

	paths := make([]string, 0, len(a.Entries))
	for p := range a.Entries {
		if pred == nil || pred(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

func normalizeArchiveLimits(limits ArchiveLimits) ArchiveLimits {
	defaults := DefaultArchiveLimits()
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = defaults.MaxFiles
	}
	if limits.MaxUncompressedSize <= 0 {
		limits.MaxUncompressedSize = defaults.MaxUncompressedSize
	}
	if limits.MaxFileSize <= 0 {
		limits.MaxFileSize = defaults.MaxFileSize
	}
	return limits
}

func normalizeArchiveName(name string) (string, error) {
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("archive entry %q contains NUL byte", name)
	}

	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || path.IsAbs(normalized) {
		return "", fmt.Errorf("archive entry %q is absolute", name)
	}

	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", fmt.Errorf("archive entry %q contains path traversal", name)
		}
	}

	clean := path.Clean(normalized)
	for _, segment := range strings.Split(clean, "/") {
		if segment == ".." {
			return "", fmt.Errorf("archive entry %q contains path traversal", name)
		}
	}

	return clean, nil
}

func zipFileSize(f *zip.File) (int64, error) {
	if f.UncompressedSize64 > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("archive entry %q is too large", f.Name)
	}
	return int64(f.UncompressedSize64), nil
}

func readZipFile(f *zip.File, maxFileSize int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	content, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxFileSize {
		return nil, fmt.Errorf("file content exceeds per-file limit")
	}
	return content, nil
}
