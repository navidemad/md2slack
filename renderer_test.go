package md2slack

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

// blockJSON is a test helper that marshals blocks to indented JSON for readable diffs.
func blockJSON(t *testing.T, blocks []slack.Block) string {
	t.Helper()
	data, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(data)
}

func TestConvert_Basic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, blocks []slack.Block)
	}{
		{
			name:  "empty input returns nil",
			input: "",
			check: func(t *testing.T, blocks []slack.Block) {
				if blocks != nil {
					t.Errorf("expected nil, got %d blocks", len(blocks))
				}
			},
		},
		{
			name:  "plain text becomes rich_text section",
			input: "Hello world",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt, ok := blocks[0].(*slack.RichTextBlock)
				if !ok {
					t.Fatalf("expected RichTextBlock, got %T", blocks[0])
				}
				if len(rt.Elements) != 1 {
					t.Fatalf("expected 1 element, got %d", len(rt.Elements))
				}
				sec, ok := rt.Elements[0].(*slack.RichTextSection)
				if !ok {
					t.Fatalf("expected RichTextSection, got %T", rt.Elements[0])
				}
				// Goldmark may split text into multiple text nodes.
				// Concatenate all text to verify content.
				var allText string
				for _, elem := range sec.Elements {
					te, ok := elem.(*slack.RichTextSectionTextElement)
					if !ok {
						t.Fatalf("expected RichTextSectionTextElement, got %T", elem)
					}
					allText += te.Text
				}
				if allText != "Hello world" {
					t.Errorf("expected %q, got %q", "Hello world", allText)
				}
			},
		},
		{
			name:  "heading becomes header block",
			input: "## Hello",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				h, ok := blocks[0].(*slack.HeaderBlock)
				if !ok {
					t.Fatalf("expected HeaderBlock, got %T", blocks[0])
				}
				if h.Text.Text != "Hello" {
					t.Errorf("expected %q, got %q", "Hello", h.Text.Text)
				}
			},
		},
		{
			name:  "divider from thematic break",
			input: "---",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				_, ok := blocks[0].(*slack.DividerBlock)
				if !ok {
					t.Fatalf("expected DividerBlock, got %T", blocks[0])
				}
			},
		},
		{
			name:  "standalone image becomes ImageBlock",
			input: "![logo](https://example.com/logo.png)",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				img, ok := blocks[0].(*slack.ImageBlock)
				if !ok {
					t.Fatalf("expected ImageBlock, got %T", blocks[0])
				}
				if img.ImageURL != "https://example.com/logo.png" {
					t.Errorf("expected URL %q, got %q", "https://example.com/logo.png", img.ImageURL)
				}
				if img.AltText != "logo" {
					t.Errorf("expected alt %q, got %q", "logo", img.AltText)
				}
			},
		},
		{
			name:  "standalone link becomes ActionBlock",
			input: "[Click here](https://example.com)",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				ab, ok := blocks[0].(*slack.ActionBlock)
				if !ok {
					t.Fatalf("expected ActionBlock, got %T", blocks[0])
				}
				if ab.Elements == nil || len(ab.Elements.ElementSet) == 0 {
					t.Fatal("expected at least 1 action element")
				}
			},
		},
		{
			name:  "fenced code block becomes preformatted",
			input: "```go\nfmt.Println(\"hello\")\n```",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt, ok := blocks[0].(*slack.RichTextBlock)
				if !ok {
					t.Fatalf("expected RichTextBlock, got %T", blocks[0])
				}
				if len(rt.Elements) != 1 {
					t.Fatalf("expected 1 element, got %d", len(rt.Elements))
				}
				_, ok = rt.Elements[0].(*slack.RichTextPreformatted)
				if !ok {
					t.Fatalf("expected RichTextPreformatted, got %T", rt.Elements[0])
				}
			},
		},
		{
			name:  "unordered list becomes RichTextBlock with list",
			input: "- Apple\n- Banana\n- Cherry",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt, ok := blocks[0].(*slack.RichTextBlock)
				if !ok {
					t.Fatalf("expected RichTextBlock, got %T", blocks[0])
				}
				if len(rt.Elements) != 1 {
					t.Fatalf("expected 1 element, got %d", len(rt.Elements))
				}
				list, ok := rt.Elements[0].(*slack.RichTextList)
				if !ok {
					t.Fatalf("expected RichTextList, got %T", rt.Elements[0])
				}
				if list.Style != slack.RTEListBullet {
					t.Errorf("expected bullet style, got %q", list.Style)
				}
				if len(list.Elements) != 3 {
					t.Errorf("expected 3 list items, got %d", len(list.Elements))
				}
			},
		},
		{
			name:  "ordered list becomes RichTextBlock with ordered list",
			input: "1. First\n2. Second",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt, ok := blocks[0].(*slack.RichTextBlock)
				if !ok {
					t.Fatalf("expected RichTextBlock, got %T", blocks[0])
				}
				list, ok := rt.Elements[0].(*slack.RichTextList)
				if !ok {
					t.Fatalf("expected RichTextList, got %T", rt.Elements[0])
				}
				if list.Style != slack.RTEListOrdered {
					t.Errorf("expected ordered style, got %q", list.Style)
				}
			},
		},
		{
			name:  "blockquote becomes rich_text with quote",
			input: "> This is a quote",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt, ok := blocks[0].(*slack.RichTextBlock)
				if !ok {
					t.Fatalf("expected RichTextBlock, got %T", blocks[0])
				}
				if len(rt.Elements) != 1 {
					t.Fatalf("expected 1 element, got %d", len(rt.Elements))
				}
				_, ok = rt.Elements[0].(*slack.RichTextQuote)
				if !ok {
					t.Fatalf("expected RichTextQuote, got %T", rt.Elements[0])
				}
			},
		},
		{
			name:  "table becomes section with code fence",
			input: "| Name | Age |\n|------|-----|\n| Alice | 30 |",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				sec, ok := blocks[0].(*slack.SectionBlock)
				if !ok {
					t.Fatalf("expected SectionBlock, got %T", blocks[0])
				}
				if sec.Text == nil {
					t.Fatal("expected text, got nil")
				}
				if sec.Text.Type != slack.MarkdownType {
					t.Errorf("expected mrkdwn, got %q", sec.Text.Type)
				}
				// Should contain code fence.
				if !strings.Contains(sec.Text.Text, "```") {
					t.Errorf("expected code fence in table text, got: %q", sec.Text.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := Convert(tt.input)
			if err != nil {
				t.Fatalf("Convert error: %v", err)
			}
			tt.check(t, blocks)
		})
	}
}

func TestConvert_InlineFormatting(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		checkFirst func(t *testing.T, elem slack.RichTextSectionElement)
	}{
		{
			name:  "bold text",
			input: "**bold**",
			checkFirst: func(t *testing.T, elem slack.RichTextSectionElement) {
				te, ok := elem.(*slack.RichTextSectionTextElement)
				if !ok {
					t.Fatalf("expected text element, got %T", elem)
				}
				if te.Text != "bold" {
					t.Errorf("expected %q, got %q", "bold", te.Text)
				}
				if te.Style == nil || !te.Style.Bold {
					t.Error("expected bold style")
				}
			},
		},
		{
			name:  "italic text",
			input: "*italic*",
			checkFirst: func(t *testing.T, elem slack.RichTextSectionElement) {
				te, ok := elem.(*slack.RichTextSectionTextElement)
				if !ok {
					t.Fatalf("expected text element, got %T", elem)
				}
				if te.Text != "italic" {
					t.Errorf("expected %q, got %q", "italic", te.Text)
				}
				if te.Style == nil || !te.Style.Italic {
					t.Error("expected italic style")
				}
			},
		},
		{
			name:  "strikethrough text",
			input: "~~struck~~",
			checkFirst: func(t *testing.T, elem slack.RichTextSectionElement) {
				te, ok := elem.(*slack.RichTextSectionTextElement)
				if !ok {
					t.Fatalf("expected text element, got %T", elem)
				}
				if te.Text != "struck" {
					t.Errorf("expected %q, got %q", "struck", te.Text)
				}
				if te.Style == nil || !te.Style.Strike {
					t.Error("expected strikethrough style")
				}
			},
		},
		{
			name:  "inline code",
			input: "`code`",
			checkFirst: func(t *testing.T, elem slack.RichTextSectionElement) {
				te, ok := elem.(*slack.RichTextSectionTextElement)
				if !ok {
					t.Fatalf("expected text element, got %T", elem)
				}
				if te.Text != "code" {
					t.Errorf("expected %q, got %q", "code", te.Text)
				}
				if te.Style == nil || !te.Style.Code {
					t.Error("expected code style")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := Convert(tt.input)
			if err != nil {
				t.Fatalf("Convert error: %v", err)
			}
			if len(blocks) < 1 {
				t.Fatalf("expected at least 1 block, got %d", len(blocks))
			}
			rt, ok := blocks[0].(*slack.RichTextBlock)
			if !ok {
				t.Fatalf("expected RichTextBlock, got %T", blocks[0])
			}
			sec, ok := rt.Elements[0].(*slack.RichTextSection)
			if !ok {
				t.Fatalf("expected RichTextSection, got %T", rt.Elements[0])
			}
			if len(sec.Elements) < 1 {
				t.Fatalf("expected at least 1 element, got %d", len(sec.Elements))
			}
			tt.checkFirst(t, sec.Elements[0])
		})
	}
}

func TestConvert_Link(t *testing.T) {
	blocks, err := Convert("See [Google](https://google.com) for more")
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	rt, ok := blocks[0].(*slack.RichTextBlock)
	if !ok {
		t.Fatalf("expected RichTextBlock, got %T", blocks[0])
	}
	sec, ok := rt.Elements[0].(*slack.RichTextSection)
	if !ok {
		t.Fatalf("expected RichTextSection, got %T", rt.Elements[0])
	}

	// Should have: "See " (text), link, " for more" (text)
	if len(sec.Elements) < 3 {
		t.Fatalf("expected at least 3 elements, got %d: %s", len(sec.Elements), blockJSON(t, blocks))
	}

	link, ok := sec.Elements[1].(*slack.RichTextSectionLinkElement)
	if !ok {
		t.Fatalf("expected RichTextSectionLinkElement at index 1, got %T", sec.Elements[1])
	}
	if link.URL != "https://google.com" {
		t.Errorf("expected URL %q, got %q", "https://google.com", link.URL)
	}
	if link.Text != "Google" {
		t.Errorf("expected text %q, got %q", "Google", link.Text)
	}
}

func TestConvert_BoldItalic(t *testing.T) {
	blocks, err := Convert("***bold italic***")
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	rt, ok := blocks[0].(*slack.RichTextBlock)
	if !ok {
		t.Fatalf("expected RichTextBlock, got %T", blocks[0])
	}
	sec, ok := rt.Elements[0].(*slack.RichTextSection)
	if !ok {
		t.Fatalf("expected RichTextSection, got %T", rt.Elements[0])
	}
	if len(sec.Elements) < 1 {
		t.Fatalf("expected at least 1 element")
	}
	te, ok := sec.Elements[0].(*slack.RichTextSectionTextElement)
	if !ok {
		t.Fatalf("expected text element, got %T", sec.Elements[0])
	}
	if te.Text != "bold italic" {
		t.Errorf("expected %q, got %q", "bold italic", te.Text)
	}
	if te.Style == nil || !te.Style.Bold || !te.Style.Italic {
		t.Errorf("expected bold+italic style, got %+v", te.Style)
	}
}

func TestConvert_NestedList(t *testing.T) {
	input := "- Parent\n  - Child A\n  - Child B\n- Another parent"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	rt, ok := blocks[0].(*slack.RichTextBlock)
	if !ok {
		t.Fatalf("expected RichTextBlock, got %T", blocks[0])
	}
	// Should have the parent list and the nested list as sibling elements
	// in the same RichTextBlock (not nested inside each other).
	if len(rt.Elements) < 2 {
		t.Fatalf("expected at least 2 elements (parent list + nested list), got %d: %s",
			len(rt.Elements), blockJSON(t, blocks))
	}
	// The top-level list
	list, ok := rt.Elements[0].(*slack.RichTextList)
	if !ok {
		t.Fatalf("expected RichTextList, got %T", rt.Elements[0])
	}
	if list.Style != slack.RTEListBullet {
		t.Errorf("expected bullet style, got %q", list.Style)
	}
	if list.Indent != 0 {
		t.Errorf("expected indent 0 for parent list, got %d", list.Indent)
	}
	if len(list.Elements) != 2 {
		t.Errorf("expected 2 items in parent list ('Parent' and 'Another parent'), got %d", len(list.Elements))
	}
	// Verify parent list item text survived conversion.
	// Goldmark splits text at spaces into multiple text nodes, so check
	// individual words rather than multi-word phrases.
	jsonStr := blockJSON(t, blocks)
	for _, text := range []string{"Parent", "Another", "parent"} {
		if !strings.Contains(jsonStr, text) {
			t.Errorf("expected %q text in output: %s", text, jsonStr)
		}
	}
	// The nested list should be a sibling element with indent 1
	nestedList, ok := rt.Elements[1].(*slack.RichTextList)
	if !ok {
		t.Fatalf("expected RichTextList for nested list, got %T", rt.Elements[1])
	}
	if nestedList.Indent != 1 {
		t.Errorf("expected indent 1 for nested list, got %d", nestedList.Indent)
	}
	if len(nestedList.Elements) != 2 {
		t.Errorf("expected 2 items in nested list, got %d", len(nestedList.Elements))
	}
	for _, text := range []string{"Child", "A", "B"} {
		if !strings.Contains(jsonStr, text) {
			t.Errorf("expected %q text in output: %s", text, jsonStr)
		}
	}
}

func TestConvert_ComplexDocument(t *testing.T) {
	input := "# Welcome\n\nHello **world**.\n\n---\n\n![banner](https://example.com/banner.png)\n\n```\ncode here\n```\n\nGoodbye."
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) < 5 {
		t.Fatalf("expected at least 5 blocks, got %d: %s", len(blocks), blockJSON(t, blocks))
	}

	// Block 0: header
	if _, ok := blocks[0].(*slack.HeaderBlock); !ok {
		t.Errorf("block[0]: expected HeaderBlock, got %T", blocks[0])
	}
	// Block 1: rich_text (paragraph)
	if _, ok := blocks[1].(*slack.RichTextBlock); !ok {
		t.Errorf("block[1]: expected RichTextBlock, got %T", blocks[1])
	}
	// Block 2: divider
	if _, ok := blocks[2].(*slack.DividerBlock); !ok {
		t.Errorf("block[2]: expected DividerBlock, got %T", blocks[2])
	}
	// Block 3: image
	if _, ok := blocks[3].(*slack.ImageBlock); !ok {
		t.Errorf("block[3]: expected ImageBlock, got %T", blocks[3])
	}
	// Block 4: rich_text preformatted
	if _, ok := blocks[4].(*slack.RichTextBlock); !ok {
		t.Errorf("block[4]: expected RichTextBlock, got %T", blocks[4])
	}
}

func TestConvert_Table(t *testing.T) {
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	sec, ok := blocks[0].(*slack.SectionBlock)
	if !ok {
		t.Fatalf("expected SectionBlock, got %T", blocks[0])
	}
	text := sec.Text.Text
	if !strings.Contains(text, "```") {
		t.Errorf("expected code fence, got: %q", text)
	}
	if !strings.Contains(text, "Name") {
		t.Errorf("expected 'Name' in table, got: %q", text)
	}
	if !strings.Contains(text, "Alice") {
		t.Errorf("expected 'Alice' in table, got: %q", text)
	}
}

func TestConvert_TableAlignment(t *testing.T) {
	input := "| Name | Score |\n|------|------:|\n| Alice | 100 |"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	sec := blocks[0].(*slack.SectionBlock)
	text := sec.Text.Text
	// Right-aligned "100" should have leading spaces.
	if !strings.Contains(text, "100") {
		t.Errorf("expected '100' in table, got: %q", text)
	}
}

func TestChunkBlocks(t *testing.T) {
	tests := []struct {
		name    string
		nBlocks int
		max     int
		want    int // number of chunks
	}{
		{"empty", 0, 50, 0},
		{"under limit", 10, 50, 1},
		{"exact limit", 50, 50, 1},
		{"over limit", 51, 50, 2},
		{"way over", 150, 50, 3},
		{"default max", 60, 0, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := make([]slack.Block, tt.nBlocks)
			for i := range blocks {
				blocks[i] = slack.NewDividerBlock()
			}
			chunks := ChunkBlocks(blocks, tt.max)
			if len(chunks) != tt.want {
				t.Errorf("expected %d chunks, got %d", tt.want, len(chunks))
			}
		})
	}
}

func TestConvert_JSONRoundTrip(t *testing.T) {
	input := "## Hello\n\n**bold** and *italic*\n\n- item 1\n- item 2"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	// Just verify it doesn't error — the slack-go types handle their own serialization.
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestConvert_HeadingWithLink(t *testing.T) {
	// Headings with links should fall back to section block with bold mrkdwn.
	blocks, err := Convert("## Click [here](https://example.com)")
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	// Should fall back to section block since heading contains a link.
	sec, ok := blocks[0].(*slack.SectionBlock)
	if !ok {
		t.Fatalf("expected SectionBlock (fallback for heading with link), got %T", blocks[0])
	}
	if sec.Text.Text != "*Click <https://example.com|here>*" {
		t.Errorf("expected bold mrkdwn fallback with link, got %q", sec.Text.Text)
	}
}

func TestConvert_MultiParagraph(t *testing.T) {
	input := "First paragraph.\n\nSecond paragraph."
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
}

func TestConvert_TaskCheckbox(t *testing.T) {
	input := "- [x] Done\n- [ ] Todo"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) < 1 {
		t.Fatalf("expected at least 1 block, got %d", len(blocks))
	}
	// Should contain checkbox emojis.
	data, _ := json.Marshal(blocks)
	text := string(data)
	if !strings.Contains(text, "☑") {
		t.Error("expected checked checkbox emoji")
	}
	if !strings.Contains(text, "☐") {
		t.Error("expected unchecked checkbox emoji")
	}
}

func TestConvert_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, blocks []slack.Block)
	}{
		{
			name:  "autolink URL",
			input: "<https://example.com>",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				sec := rt.Elements[0].(*slack.RichTextSection)
				found := false
				for _, elem := range sec.Elements {
					if link, ok := elem.(*slack.RichTextSectionLinkElement); ok {
						if link.URL == "https://example.com" {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected link element with URL, got: %s", blockJSON(t, blocks))
				}
			},
		},
		{
			name:  "autolink email",
			input: "<user@example.com>",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				sec := rt.Elements[0].(*slack.RichTextSection)
				found := false
				for _, elem := range sec.Elements {
					if link, ok := elem.(*slack.RichTextSectionLinkElement); ok {
						if link.URL == "mailto:user@example.com" {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected mailto link element, got: %s", blockJSON(t, blocks))
				}
			},
		},
		{
			name:  "indented code block",
			input: "    code line 1\n    code line 2",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				_, ok := rt.Elements[0].(*slack.RichTextPreformatted)
				if !ok {
					t.Fatalf("expected RichTextPreformatted, got %T", rt.Elements[0])
				}
			},
		},
		{
			name:  "heading over 150 chars falls back to section",
			input: "## " + strings.Repeat("A", 160),
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				sec, ok := blocks[0].(*slack.SectionBlock)
				if !ok {
					t.Fatalf("expected SectionBlock fallback, got %T", blocks[0])
				}
				if sec.Text.Type != slack.MarkdownType {
					t.Errorf("expected mrkdwn type, got %q", sec.Text.Type)
				}
				if !strings.HasPrefix(sec.Text.Text, "*") {
					t.Errorf("expected bold mrkdwn wrapper, got %q", sec.Text.Text[:10])
				}
			},
		},
		{
			name:  "heading with image falls back to section",
			input: "## Title ![img](https://example.com/img.png)",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				_, ok := blocks[0].(*slack.SectionBlock)
				if !ok {
					t.Fatalf("expected SectionBlock fallback for heading with image, got %T", blocks[0])
				}
			},
		},
		{
			name:  "heading link preserves URL in mrkdwn",
			input: "## Visit [Google](https://google.com) today",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				sec := blocks[0].(*slack.SectionBlock)
				if !strings.Contains(sec.Text.Text, "https://google.com") {
					t.Errorf("expected URL preserved in mrkdwn, got %q", sec.Text.Text)
				}
				if !strings.Contains(sec.Text.Text, "<https://google.com|Google>") {
					t.Errorf("expected mrkdwn link syntax, got %q", sec.Text.Text)
				}
			},
		},
		{
			name:  "nested blockquotes flatten into single quote",
			input: "> outer\n> > inner",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				_, ok := rt.Elements[0].(*slack.RichTextQuote)
				if !ok {
					t.Fatalf("expected RichTextQuote, got %T", rt.Elements[0])
				}
			},
		},
		{
			name:  "multi-paragraph blockquote",
			input: "> First paragraph.\n>\n> Second paragraph.",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				quote, ok := rt.Elements[0].(*slack.RichTextQuote)
				if !ok {
					t.Fatalf("expected RichTextQuote, got %T", rt.Elements[0])
				}
				// Should have elements from both paragraphs with separator.
				if len(quote.Elements) < 3 {
					t.Errorf("expected at least 3 elements (two paragraphs + separator), got %d", len(quote.Elements))
				}
			},
		},
		{
			name:  "mixed list types ordered inside unordered",
			input: "- Fruit\n  1. Apple\n  2. Banana\n- Veggies",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				list, ok := rt.Elements[0].(*slack.RichTextList)
				if !ok {
					t.Fatalf("expected RichTextList, got %T", rt.Elements[0])
				}
				if list.Style != slack.RTEListBullet {
					t.Errorf("expected outer bullet list, got %q", list.Style)
				}
				// Verify parent item text survived.
				jsonStr := blockJSON(t, blocks)
				for _, text := range []string{"Fruit", "Apple", "Banana", "Veggies"} {
					if !strings.Contains(jsonStr, text) {
						t.Errorf("expected %q in output: %s", text, jsonStr)
					}
				}
				// Nested ordered list should be a sibling element in the
				// RichTextBlock, not inside the parent list's Elements.
				hasNested := false
				for _, elem := range rt.Elements[1:] {
					if nested, ok := elem.(*slack.RichTextList); ok {
						if nested.Style == slack.RTEListOrdered && nested.Indent == 1 {
							hasNested = true
						}
					}
				}
				if !hasNested {
					t.Errorf("expected nested ordered list as sibling element, got: %s", jsonStr)
				}
			},
		},
		{
			name:  "soft line break becomes newline",
			input: "line one\nline two",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				data, _ := json.Marshal(blocks)
				if !strings.Contains(string(data), "\\n") {
					t.Errorf("expected newline in output, got: %s", string(data))
				}
			},
		},
		{
			name:  "hard line break",
			input: "line one  \nline two",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				data, _ := json.Marshal(blocks)
				if !strings.Contains(string(data), "\\n") {
					t.Errorf("expected newline in output, got: %s", string(data))
				}
			},
		},
		{
			name:  "whitespace-only input returns nil",
			input: "   \n\t\n   ",
			check: func(t *testing.T, blocks []slack.Block) {
				if blocks != nil {
					t.Errorf("expected nil for whitespace-only input, got %d blocks", len(blocks))
				}
			},
		},
		{
			name:  "standalone image with empty alt text",
			input: "![](https://example.com/img.png)",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				img, ok := blocks[0].(*slack.ImageBlock)
				if !ok {
					t.Fatalf("expected ImageBlock, got %T", blocks[0])
				}
				// Empty alt should be replaced with space for Slack API.
				if img.AltText != " " {
					t.Errorf("expected alt text %q for empty alt, got %q", " ", img.AltText)
				}
			},
		},
		{
			name:  "inline image becomes link fallback",
			input: "See ![logo](https://example.com/logo.png) here",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				rt := blocks[0].(*slack.RichTextBlock)
				sec := rt.Elements[0].(*slack.RichTextSection)
				found := false
				for _, elem := range sec.Elements {
					if link, ok := elem.(*slack.RichTextSectionLinkElement); ok {
						if link.URL == "https://example.com/logo.png" && link.Text == "logo" {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected inline image to fall back to link, got: %s", blockJSON(t, blocks))
				}
			},
		},
		{
			name:  "code block JSON serializes as rich_text_preformatted",
			input: "```\nhello\n```",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				data, _ := json.Marshal(blocks)
				jsonStr := string(data)
				if !strings.Contains(jsonStr, "rich_text_preformatted") {
					t.Errorf("expected 'rich_text_preformatted' in JSON, got: %s", jsonStr)
				}
			},
		},
		{
			name:  "heading with code span",
			input: "## Use `fmt.Println`",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				h, ok := blocks[0].(*slack.HeaderBlock)
				if !ok {
					t.Fatalf("expected HeaderBlock, got %T", blocks[0])
				}
				if !strings.Contains(h.Text.Text, "fmt.Println") {
					t.Errorf("expected code span text in heading, got %q", h.Text.Text)
				}
			},
		},
		{
			name:  "heading with autolink falls back to section with mrkdwn link",
			input: "## See <https://example.com>",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				sec, ok := blocks[0].(*slack.SectionBlock)
				if !ok {
					t.Fatalf("expected SectionBlock fallback, got %T", blocks[0])
				}
				if !strings.Contains(sec.Text.Text, "<https://example.com|") {
					t.Errorf("expected mrkdwn link in heading fallback, got %q", sec.Text.Text)
				}
			},
		},
		{
			name:  "action block has unique IDs",
			input: "[Link A](https://a.com)\n\n[Link B](https://b.com)",
			check: func(t *testing.T, blocks []slack.Block) {
				if len(blocks) != 2 {
					t.Fatalf("expected 2 blocks, got %d: %s", len(blocks), blockJSON(t, blocks))
				}
				a := blocks[0].(*slack.ActionBlock)
				b := blocks[1].(*slack.ActionBlock)
				if a.BlockID == b.BlockID {
					t.Errorf("expected unique block IDs, both are %q", a.BlockID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := Convert(tt.input)
			if err != nil {
				t.Fatalf("Convert error: %v", err)
			}
			tt.check(t, blocks)
		})
	}
}

// TestConvert_NestedListJSON verifies that nested lists produce valid Slack Block Kit JSON.
// Slack rejects rich_text_list nested inside rich_text_list elements — nested lists
// must be sibling elements within the same rich_text block with incremented indent.
func TestConvert_NestedListJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectWords []string // individual words that must appear in JSON output
	}{
		{
			name:        "multiple nested items",
			input:       "- Item one\n- Item two\n  - Nested A\n  - Nested B\n- Item three",
			expectWords: []string{"Item", "one", "two", "Nested", "three"},
		},
		{
			name:        "single nested item",
			input:       "- Item one\n- Item two\n  - Nested\n- Item three",
			expectWords: []string{"Item", "one", "two", "Nested", "three"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := Convert(tt.input)
			if err != nil {
				t.Fatalf("Convert error: %v", err)
			}

			data, err := json.Marshal(blocks)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			jsonStr := string(data)

			// Verify no empty text elements.
			if strings.Contains(jsonStr, `"text":""`) {
				t.Errorf("JSON contains empty text element: %s", jsonStr)
			}

			// Verify all expected text content survived conversion.
			for _, word := range tt.expectWords {
				if !strings.Contains(jsonStr, word) {
					t.Errorf("expected %q in JSON output: %s", word, blockJSON(t, blocks))
				}
			}

			// Verify structure: single RichTextBlock with multiple list elements.
			if len(blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(blocks))
			}
			rt := blocks[0].(*slack.RichTextBlock)

			// Should have parent list (indent 0) and nested list (indent 1) as siblings.
			if len(rt.Elements) < 2 {
				t.Fatalf("expected at least 2 elements, got %d: %s", len(rt.Elements), blockJSON(t, blocks))
			}

			// All elements should be RichTextList — no nesting.
			for i, elem := range rt.Elements {
				list, ok := elem.(*slack.RichTextList)
				if !ok {
					t.Errorf("element[%d]: expected RichTextList, got %T", i, elem)
					continue
				}
				if len(list.Elements) == 0 {
					t.Errorf("element[%d]: RichTextList has zero items", i)
				}
				// Verify list items are only RichTextSection (no nested lists).
				for j, item := range list.Elements {
					if _, ok := item.(*slack.RichTextSection); !ok {
						t.Errorf("element[%d].items[%d]: expected RichTextSection, got %T", i, j, item)
					}
				}
			}
		})
	}
}

// TestConvert_DeeplyNestedList verifies 3+ levels of nesting produce correct
// sibling elements with incrementing indent values.
func TestConvert_DeeplyNestedList(t *testing.T) {
	input := "- Level 0\n  - Level 1\n    - Level 2a\n    - Level 2b\n  - Level 1 again"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	rt, ok := blocks[0].(*slack.RichTextBlock)
	if !ok {
		t.Fatalf("expected RichTextBlock, got %T", blocks[0])
	}

	// Verify all text content survived conversion.
	// Goldmark splits text at spaces, so check individual words.
	jsonStr := blockJSON(t, blocks)
	for _, word := range []string{"Level", "2a", "2b", "again"} {
		if !strings.Contains(jsonStr, word) {
			t.Errorf("expected %q in output: %s", word, jsonStr)
		}
	}

	// Should have 3 sibling RichTextList elements (indent 0, 1, 2).
	if len(rt.Elements) < 3 {
		t.Fatalf("expected at least 3 elements (indent 0, 1, 2), got %d: %s",
			len(rt.Elements), jsonStr)
	}

	// All elements should be RichTextList with no nested lists inside.
	indentsSeen := map[int]bool{}
	for i, elem := range rt.Elements {
		list, ok := elem.(*slack.RichTextList)
		if !ok {
			t.Errorf("element[%d]: expected RichTextList, got %T", i, elem)
			continue
		}
		indentsSeen[list.Indent] = true
		if len(list.Elements) == 0 {
			t.Errorf("element[%d]: RichTextList at indent %d has zero items", i, list.Indent)
		}
		for j, item := range list.Elements {
			if _, ok := item.(*slack.RichTextSection); !ok {
				t.Errorf("element[%d].items[%d]: expected RichTextSection, got %T", i, j, item)
			}
		}
	}

	for _, indent := range []int{0, 1, 2} {
		if !indentsSeen[indent] {
			t.Errorf("expected a list at indent %d, got indents: %v", indent, indentsSeen)
		}
	}
}

// TestConvert_MixedDeeplyNestedList verifies 3 levels with mixed ordered/unordered types.
func TestConvert_MixedDeeplyNestedList(t *testing.T) {
	// 5 spaces needed to nest under "1. " (3 chars) + 2 for the parent bullet indent.
	input := "- Bullet\n  1. Ordered\n     - Nested bullet"
	blocks, err := Convert(input)
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %s", len(blocks), blockJSON(t, blocks))
	}
	rt := blocks[0].(*slack.RichTextBlock)

	// Verify text content — goldmark splits at spaces, so check individual words.
	jsonStr := blockJSON(t, blocks)
	for _, word := range []string{"Bullet", "Ordered", "Nested"} {
		if !strings.Contains(jsonStr, word) {
			t.Errorf("expected %q in output: %s", word, jsonStr)
		}
	}

	// Verify we have lists at indent 0 (bullet), 1 (ordered), 2 (bullet).
	type listInfo struct {
		style  slack.RichTextListElementType
		indent int
	}
	var lists []listInfo
	for _, elem := range rt.Elements {
		if list, ok := elem.(*slack.RichTextList); ok {
			lists = append(lists, listInfo{style: list.Style, indent: list.Indent})
		}
	}
	if len(lists) < 3 {
		t.Fatalf("expected at least 3 list elements, got %d: %s", len(lists), jsonStr)
	}
	if lists[0].style != slack.RTEListBullet || lists[0].indent != 0 {
		t.Errorf("expected bullet at indent 0, got %q at %d", lists[0].style, lists[0].indent)
	}
	if lists[1].style != slack.RTEListOrdered || lists[1].indent != 1 {
		t.Errorf("expected ordered at indent 1, got %q at %d", lists[1].style, lists[1].indent)
	}
	if lists[2].style != slack.RTEListBullet || lists[2].indent != 2 {
		t.Errorf("expected bullet at indent 2, got %q at %d", lists[2].style, lists[2].indent)
	}
}

// FuzzConvert verifies that Convert never panics on arbitrary input.
func FuzzConvert(f *testing.F) {
	f.Add("")
	f.Add("Hello world")
	f.Add("## Heading\n**bold** and [link](https://example.com)")
	f.Add("```\ncode\n```")
	f.Add("![img](https://img.com/pic.png)")
	f.Add("~~~\ncode\n~~~")
	f.Add("> block quote with **bold** & stuff")
	f.Add("| A | B |\n|---|---|\n| 1 | 2 |")
	f.Add("- [x] done\n- [ ] todo")
	f.Add("***bold italic***")
	f.Add("~~strikethrough~~")

	f.Fuzz(func(t *testing.T, input string) {
		_, err := Convert(input)
		if err != nil {
			t.Errorf("Convert panicked or errored on %q: %v", input, err)
		}
	})
}
