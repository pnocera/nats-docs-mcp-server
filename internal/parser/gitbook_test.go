package parser

import (
	"strings"
	"testing"
)

func TestPreprocess_HintInfo(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`{% hint style="info" %}foo{% endhint %}`))))
	if got != "Info:\nfoo" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_HintWarning(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`{% hint style="warning" %}foo{% endhint %}`))))
	if got != "Warning:\nfoo" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_HintSuccess(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`{% hint style="success" %}foo{% endhint %}`))))
	if got != "Note:\nfoo" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_TabsTitle(t *testing.T) {
	got := string(PreprocessGitBook([]byte(`{% tab title="Go" %}body{% endtab %}`)))
	if !strings.Contains(got, "### Go") || !strings.Contains(got, "body") {
		t.Fatalf("expected tab title and body, got %q", got)
	}
}

func TestPreprocess_TabsEmbedded(t *testing.T) {
	got := string(PreprocessGitBook([]byte(`{% tabs %}{% tab title="Go" %}body{% endtab %}{% endtabs %}`)))
	if strings.Contains(got, "{% tabs %}") || strings.Contains(got, "{% endtabs %}") {
		t.Fatalf("tab wrappers were not removed: %q", got)
	}
}

func TestPreprocess_Embed(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`{% embed url="https://example.com" %}`))))
	if got != "Embedded: https://example.com" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_UnknownDirective(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`before {% foo bar=baz %} after`))))
	if got != "before  after" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_CodeFenceUntouched(t *testing.T) {
	input := "```go\n{% hint %}\n```\n"
	got := string(PreprocessGitBook([]byte(input)))
	if got != input {
		t.Fatalf("expected fenced block unchanged, got %q", got)
	}
}

func TestPreprocess_TildeFenceUntouched(t *testing.T) {
	input := "~~~go\n{% hint %}\n~~~\n"
	got := string(PreprocessGitBook([]byte(input)))
	if got != input {
		t.Fatalf("expected fenced block unchanged, got %q", got)
	}
}

func TestPreprocess_DirectiveBeforeFence(t *testing.T) {
	input := "{% hint %}\n```\n{% hint %}\n```\n"
	got := string(PreprocessGitBook([]byte(input)))
	if !strings.HasPrefix(got, "Note:\n") {
		t.Fatalf("expected directive before fence to be rewritten, got %q", got)
	}
	if !strings.Contains(got, "```\n{% hint %}\n```") {
		t.Fatalf("expected fenced directive to remain literal, got %q", got)
	}
}

func TestPreprocess_DirectiveAfterFence(t *testing.T) {
	input := "```\n{% hint %}\n```\n{% hint %}\n"
	got := string(PreprocessGitBook([]byte(input)))
	if !strings.HasSuffix(got, "Note:\n\n") {
		t.Fatalf("expected directive after fence to be rewritten, got %q", got)
	}
}

func TestPreprocess_DirectiveInsideFence(t *testing.T) {
	input := "```text\n{% hint style=\"info\" %}\n{% endhint %}\n```\n"
	got := string(PreprocessGitBook([]byte(input)))
	if got != input {
		t.Fatalf("expected fenced directives unchanged, got %q", got)
	}
}

func TestPreprocess_MismatchedFenceMarker(t *testing.T) {
	input := "```text\n~~~\n{% hint %}\n```\n"
	got := string(PreprocessGitBook([]byte(input)))
	if got != input {
		t.Fatalf("expected mismatched marker inside fence to be content, got %q", got)
	}
}

func TestPreprocess_FourBacktickFence(t *testing.T) {
	input := "````text\n```\n{% hint %}\n````\n"
	got := string(PreprocessGitBook([]byte(input)))
	if got != input {
		t.Fatalf("expected shorter backtick line inside fence to be content, got %q", got)
	}
}

func TestPreprocess_UnknownDirectiveContainingPercent(t *testing.T) {
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(`before {% foo style="50%" %} after`))))
	if got != "before  after" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_HTMLFigure(t *testing.T) {
	input := `<figure><img alt="A logo" src="x"/><figcaption>caption</figcaption></figure>`
	got := strings.TrimSpace(string(PreprocessGitBook([]byte(input))))
	if got != "A logo\ncaption" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreprocess_GoldmarkRoundtrip(t *testing.T) {
	input := []byte("# Title\n\n{% hint style=\"info\" %}\nCareful.\n{% endhint %}\n")
	doc, err := ParseMarkdown(PreprocessGitBook(input), "fixture.md")
	if err != nil {
		t.Fatalf("ParseMarkdown returned error: %v", err)
	}
	if len(doc.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
}
