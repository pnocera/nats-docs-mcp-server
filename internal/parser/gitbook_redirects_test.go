package parser

import "testing"

func TestParseGitBookRedirects_Basic(t *testing.T) {
	redirects, err := ParseGitBookRedirects([]byte("redirects:\n  old/path: ./new/path.md\n"))
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if len(redirects) != 1 {
		t.Fatalf("expected one redirect, got %d", len(redirects))
	}
	if redirects[0] != (Redirect{From: "old/path", To: "new/path.md"}) {
		t.Fatalf("unexpected redirect: %+v", redirects[0])
	}
}

func TestParseGitBookRedirects_StripDot(t *testing.T) {
	redirects, err := ParseGitBookRedirects([]byte("redirects:\n  old: ./legacy/foo.md\n"))
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if len(redirects) != 1 || redirects[0].To != "legacy/foo.md" {
		t.Fatalf("unexpected redirects: %+v", redirects)
	}
}

func TestParseGitBookRedirects_Empty(t *testing.T) {
	redirects, err := ParseGitBookRedirects(nil)
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if redirects != nil {
		t.Fatalf("expected nil redirects, got %+v", redirects)
	}
}

func TestParseGitBookRedirects_NoBlock(t *testing.T) {
	redirects, err := ParseGitBookRedirects([]byte("root: ./\nstructure:\n  readme: README.md\n"))
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if redirects != nil {
		t.Fatalf("expected nil redirects, got %+v", redirects)
	}
}

func TestParseGitBookRedirects_RejectsTraversal(t *testing.T) {
	_, err := ParseGitBookRedirects([]byte("redirects:\n  old: ../../etc/passwd\n"))
	if err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestParseGitBookRedirects_SortedOut(t *testing.T) {
	redirects, err := ParseGitBookRedirects([]byte("redirects:\n  z: ./z.md\n  a: ./a.md\n"))
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if len(redirects) != 2 || redirects[0].From != "a" || redirects[1].From != "z" {
		t.Fatalf("expected sorted redirects, got %+v", redirects)
	}
}

func TestParseGitBookRedirects_RealSample(t *testing.T) {
	const sample = `redirects:
  nats-tools/nas: ./legacy/nas/README.md
  nats-tools/nas/dir_store: ./legacy/nas/dir_store.md
  nats-tools/nas/inspecting_jwts: ./legacy/nas/inspecting_jwts.md
  nats-tools/nas/nas_conf: ./legacy/nas/nas_conf.md
  nats-tools/nas/notifications: ./legacy/nas/notifications.md
  developing-with-nats-streaming/acks: ./legacy/stan/developing-with-nats-streaming/acks.md
  developing-with-nats-streaming/connecting: ./legacy/stan/developing-with-nats-streaming/connecting.md
  developing-with-nats-streaming/durables: ./legacy/stan/developing-with-nats-streaming/durables.md
  developing-with-nats-streaming/protocol: ./legacy/stan/developing-with-nats-streaming/protocol.md
  developing-with-nats-streaming/publishing: ./legacy/stan/developing-with-nats-streaming/publishing.md
`
	redirects, err := ParseGitBookRedirects([]byte(sample))
	if err != nil {
		t.Fatalf("ParseGitBookRedirects returned error: %v", err)
	}
	if len(redirects) != 10 {
		t.Fatalf("expected 10 redirects, got %d", len(redirects))
	}
}
