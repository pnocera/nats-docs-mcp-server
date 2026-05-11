package server

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func buildFixtureArchive(t *testing.T) string {
	t.Helper()
	return buildArchiveFromEntries(t, map[string]string{
		"SUMMARY.md":           "# Table of contents\n\n* [Welcome](README.md)\n* [Overview](overview.md)\n* [A](nested/a.md)\n* [B](nested/b/README.md)\n* [Empty](empty.md)\n",
		"README.md":            "# Welcome\n\nRoot page.\n",
		"overview.md":          "# Overview\n\n{% hint style=\"info\" %}\nBe careful.\n{% endhint %}\n",
		"nested/a.md":          "# A\n\n{% tabs %}{% tab title=\"Go\" %}\nGo body\n{% endtab %}{% endtabs %}\n",
		"nested/b/README.md":   "# Nested B\n",
		"empty.md":             "",
		".bookignore":          "_book/\n_docs/\n_examples/\n_tools/\ndocs/\n\nMakefile\nbuilding_the_book.md\n",
		".gitbook.yaml":        "redirects:\n  nats-tools/nas: ./legacy/nas/README.md\n  overview-old: ./overview.md\n",
		"legacy/nas/README.md": "# Legacy NAS\n",
		"zh-cn/placeholder.md": "# Chinese placeholder\n",
		"docs/generated.md":    "# Generated\n",
		"_examples/sample.md":  "# Example\n",
		"orphan.md":            "# Orphan\n",
		"building_the_book.md": "# Build\n",
		"subdir/Makefile":      "build:\n",
		"notes..old.md":        "# Legal double-dot basename\n",
	})
}

func buildArchiveFromEntries(t *testing.T, entries map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "nats-fixture.zip")

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create fixture archive: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, body := range entries {
		w, err := writer.Create("nats-fixture/" + name)
		if err != nil {
			t.Fatalf("create archive entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write archive entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture archive: %v", err)
	}
	return archivePath
}
