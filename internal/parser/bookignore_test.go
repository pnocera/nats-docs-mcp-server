package parser

import "testing"

func TestParseBookIgnore_Comments(t *testing.T) {
	ignore := ParseBookIgnore([]byte("\n# comment\n\nMakefile\n"))
	if !ignore.Match("Makefile") {
		t.Fatal("expected Makefile to match")
	}
	if ignore.Match("README.md") {
		t.Fatal("did not expect README.md to match")
	}
}

func TestParseBookIgnore_DirPrefixes(t *testing.T) {
	ignore := ParseBookIgnore([]byte("_book/\n"))
	if !ignore.Match("_book/foo.html") {
		t.Fatal("expected _book/foo.html to match")
	}
	if !ignore.Match("_book/sub/x.md") {
		t.Fatal("expected _book/sub/x.md to match")
	}
}

func TestParseBookIgnore_ExactNames(t *testing.T) {
	ignore := ParseBookIgnore([]byte("Makefile\n"))
	if !ignore.Match("Makefile") {
		t.Fatal("expected root Makefile to match")
	}
	if !ignore.Match("subdir/Makefile") {
		t.Fatal("expected nested Makefile to match")
	}
}

func TestParseBookIgnore_NoFalsePositives(t *testing.T) {
	ignore := ParseBookIgnore([]byte("docs/\n"))
	if !ignore.Match("docs/x") {
		t.Fatal("expected docs/x to match")
	}
	if ignore.Match("running-a-nats-service/docs/x") {
		t.Fatal("did not expect nested non-prefix docs path to match")
	}
	if ignore.Match("running-a-nats-service/x") {
		t.Fatal("did not expect unrelated path to match")
	}
}
