package parser

import "testing"

func TestParseSummary_BasicLinks(t *testing.T) {
	links, err := ParseSummary([]byte("* [Welcome](README.md)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected one link, got %d", len(links))
	}
	if links[0] != (SummaryLink{Title: "Welcome", Path: "README.md"}) {
		t.Fatalf("unexpected link: %+v", links[0])
	}
}

func TestParseSummary_EscapedUnderscore(t *testing.T) {
	links, err := ParseSummary([]byte(`* [X](release\_notes/whats\_new.md)`))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 1 || links[0].Path != "release_notes/whats_new.md" {
		t.Fatalf("unexpected links: %+v", links)
	}
}

func TestParseSummary_StripsLeadingDot(t *testing.T) {
	links, err := ParseSummary([]byte("* [Welcome](./README.md)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 1 || links[0].Path != "README.md" {
		t.Fatalf("unexpected links: %+v", links)
	}
}

func TestParseSummary_PreservesOrder(t *testing.T) {
	links, err := ParseSummary([]byte("* [A](a.md)\n* [B](b.md)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 2 || links[0].Path != "a.md" || links[1].Path != "b.md" {
		t.Fatalf("unexpected order: %+v", links)
	}
}

func TestParseSummary_DedupesByPath(t *testing.T) {
	links, err := ParseSummary([]byte("* [A](a.md)\n* [A again](a.md)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 1 || links[0].Title != "A" {
		t.Fatalf("unexpected links: %+v", links)
	}
}

func TestParseSummary_SkipsExternal(t *testing.T) {
	links, err := ParseSummary([]byte("* [External](https://example.com/x.md)\n* [Anchor](#anchor)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no links, got %+v", links)
	}
}

func TestParseSummary_SkipsNonMD(t *testing.T) {
	links, err := ParseSummary([]byte("* [Image](something.svg)\n"))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no links, got %+v", links)
	}
}

func TestParseSummary_RealArchiveExcerpt(t *testing.T) {
	const summary = `# Table of contents

* [Welcome](README.md)

## Release Notes

* [What's New!](release\_notes/whats\_new.md)
  * [NATS 2.14](release\_notes/whats\_new\_214.md)
  * [NATS 2.12](release\_notes/whats\_new\_212.md)
  * [NATS 2.11](release\_notes/whats\_new\_211.md)
  * [NATS 2.10](release\_notes/whats\_new\_210.md)
  * [NATS 2.2](release\_notes/whats\_new\_22.md)
  * [NATS 2.0](release\_notes/whats\_new\_20.md)

## NATS Concepts

* [Overview](overview.md)
  * [Compare NATS](nats-concepts/overview/compare-nats.md)
* [What is NATS](nats-concepts/what-is-nats/README.md)
  * [Walkthrough Setup](nats-concepts/what-is-nats/walkthrough\_setup.md)
* [Subject-Based Messaging](nats-concepts/subjects.md)
`
	links, err := ParseSummary([]byte(summary))
	if err != nil {
		t.Fatalf("ParseSummary returned error: %v", err)
	}
	if len(links) < 10 {
		t.Fatalf("expected at least 10 links, got %d", len(links))
	}
	if links[0].Path != "README.md" {
		t.Fatalf("expected first link README.md, got %+v", links[0])
	}
}
