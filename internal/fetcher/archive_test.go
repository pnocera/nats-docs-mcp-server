package fetcher

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLocalArchive_HappyPath(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/README.md", body: "root"},
		{name: "repo-master/a/b.md", body: "nested"},
		{name: "repo-master/c.md", body: "third"},
	})

	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	if archive.Root != "repo-master" {
		t.Fatalf("expected root repo-master, got %q", archive.Root)
	}
	entry, ok := archive.Get("a/b.md")
	if !ok {
		t.Fatal("expected nested entry")
	}
	if string(entry.Content) != "nested" {
		t.Fatalf("expected nested content, got %q", string(entry.Content))
	}
}

func TestOpenLocalArchive_RejectsAbsolute(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "/etc/passwd", body: "x"}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestOpenLocalArchive_RejectsTraversal(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "repo-master/../etc/passwd", body: "x"}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal error, got %v", err)
	}
}

func TestOpenLocalArchive_AllowsDoubleDotInBasename(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "repo-master/notes..old.md", body: "x"}})
	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	if _, ok := archive.Get("notes..old.md"); !ok {
		t.Fatal("expected double-dot basename entry")
	}
}

func TestOpenLocalArchive_RejectsBackslashAbsolute(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: `\windows\path`, body: "x"}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestOpenLocalArchive_RejectsMixedRoots(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/README.md", body: "x"},
		{name: "other/README.md", body: "y"},
	})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "multiple root") {
		t.Fatalf("expected mixed root error, got %v", err)
	}
}

func TestOpenLocalArchive_RejectsNul(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "repo-master/a\x00.md", body: "x"}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("expected NUL error, got %v", err)
	}
}

func TestOpenLocalArchive_FileCountLimit(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/1.md", body: "1"},
		{name: "repo-master/2.md", body: "2"},
		{name: "repo-master/3.md", body: "3"},
	})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{MaxFiles: 2})
	if err == nil || !strings.Contains(err.Error(), "file count") {
		t.Fatalf("expected file count error, got %v", err)
	}
}

func TestOpenLocalArchive_TotalSizeLimit(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/1.md", body: strings.Repeat("a", 50)},
		{name: "repo-master/2.md", body: strings.Repeat("b", 50)},
		{name: "repo-master/3.md", body: strings.Repeat("c", 50)},
	})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{MaxUncompressedSize: 100})
	if err == nil || !strings.Contains(err.Error(), "uncompressed size") {
		t.Fatalf("expected total size error, got %v", err)
	}
}

func TestOpenLocalArchive_PerFileSizeLimit(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "repo-master/large.md", body: strings.Repeat("a", 100)}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{MaxFileSize: 50})
	if err == nil || !strings.Contains(err.Error(), "per-file") {
		t.Fatalf("expected per-file size error, got %v", err)
	}
}

func TestOpenLocalArchive_SkipsDirs(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/dir/", dir: true},
		{name: "repo-master/dir/file.md", body: "x"},
	})
	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	if len(archive.Entries) != 1 {
		t.Fatalf("expected one regular entry, got %d", len(archive.Entries))
	}
}

func TestOpenLocalArchive_SkipsSymlinks(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/file.md", body: "x"},
		{name: "repo-master/link.md", body: "target", mode: os.ModeSymlink | 0777},
	})
	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	if _, ok := archive.Get("link.md"); ok {
		t.Fatal("expected symlink entry to be skipped")
	}
}

func TestOpenLocalArchive_MissingRoot(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "README.md", body: "x"}})
	_, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err == nil || !strings.Contains(err.Error(), "single root") {
		t.Fatalf("expected missing root error, got %v", err)
	}
}

func TestArchive_Get(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{{name: "repo-master/README.md", body: "root"}})
	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	entry, ok := archive.Get("./README.md")
	if !ok {
		t.Fatal("expected README entry")
	}
	if entry.Path != "README.md" || string(entry.Content) != "root" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestArchive_List(t *testing.T) {
	archivePath := buildArchive(t, []zipEntry{
		{name: "repo-master/b.md", body: "b"},
		{name: "repo-master/a.md", body: "a"},
		{name: "repo-master/c.txt", body: "c"},
	})
	archive, err := OpenLocalArchive(context.Background(), archivePath, ArchiveLimits{})
	if err != nil {
		t.Fatalf("OpenLocalArchive returned error: %v", err)
	}
	paths := archive.List(func(p string) bool { return strings.HasSuffix(p, ".md") })
	want := []string{"a.md", "b.md"}
	if len(paths) != len(want) {
		t.Fatalf("expected %v, got %v", want, paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, paths)
		}
	}
}

type zipEntry struct {
	name string
	body string
	dir  bool
	mode os.FileMode
}

func buildArchive(t *testing.T, entries []zipEntry) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "fixture.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		if entry.dir {
			header.SetMode(os.ModeDir | 0755)
		} else if entry.mode != 0 {
			header.SetMode(entry.mode)
		} else {
			header.SetMode(0644)
		}

		w, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", entry.name, err)
		}
		if !entry.dir {
			if _, err := w.Write([]byte(entry.body)); err != nil {
				t.Fatalf("write zip entry %q: %v", entry.name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return archivePath
}
