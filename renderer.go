package md2slack

import (
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var (
	parserOnce sync.Once
	parserInst goldmark.Markdown
)

func getParser() goldmark.Markdown {
	parserOnce.Do(func() {
		parserInst = goldmark.New(
			goldmark.WithExtensions(extension.GFM),
		)
	})
	return parserInst
}

// Convert parses a Markdown string and returns Slack Block Kit blocks.
//
// It uses goldmark with GFM extensions to parse the markdown into an AST,
// then walks the tree to build structured Slack blocks. The resulting blocks
// can be used directly with the slack-go library to post messages.
//
// Supported markdown features:
//   - Headings (# through ######) → HeaderBlock (plain_text, max 150 chars)
//   - Bold (**text** / __text__) → RichTextSectionTextStyle{Bold: true}
//   - Italic (*text* / _text_) → RichTextSectionTextStyle{Italic: true}
//   - Strikethrough (~~text~~) → RichTextSectionTextStyle{Strike: true}
//   - Inline code (`code`) → RichTextSectionTextStyle{Code: true}
//   - Links ([text](url)) → RichTextSectionLinkElement
//   - Images (![alt](url)) → ImageBlock (standalone) or link element (inline)
//   - Fenced code blocks (```) → RichTextBlock with RichTextPreformatted
//   - Blockquotes (> text) → RichTextBlock with RichTextQuote
//   - Ordered lists (1. item) → RichTextBlock with RichTextList (ordered)
//   - Unordered lists (- item) → RichTextBlock with RichTextList (bullet)
//   - Nested lists → RichTextList with indent levels
//   - Tables (GFM) → TableBlock with rich text cells
//   - Horizontal rules (---) → DividerBlock
//   - Standalone links ([text](url) alone) → ActionBlock with button
//   - Task checkboxes (- [x] item) → checkbox emoji text
//
// An empty string returns nil and no error.
func Convert(markdown string) ([]slack.Block, error) {
	if strings.TrimSpace(markdown) == "" {
		return nil, nil
	}

	source := []byte(markdown)

	md := getParser()
	doc := md.Parser().Parse(text.NewReader(source))

	ctx := &renderContext{
		source: source,
	}

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch v := n.(type) {
		// Block nodes.
		case *ast.Document:
			ctx.handleDocument(v, entering)
		case *ast.Heading:
			ctx.handleHeading(v, entering)
		case *ast.Paragraph:
			ctx.handleParagraph(v, entering)
		case *ast.Blockquote:
			ctx.handleBlockquote(v, entering)
		case *ast.FencedCodeBlock:
			ctx.handleFencedCodeBlock(v, entering)
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.CodeBlock:
			ctx.handleCodeBlock(v, entering)
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.List:
			ctx.handleList(v, entering)
		case *ast.ListItem:
			ctx.handleListItem(v, entering)
		case *ast.ThematicBreak:
			ctx.handleThematicBreak(v, entering)
		case *ast.HTMLBlock:
			ctx.handleHTMLBlock(v, entering)
		case *ast.TextBlock:
			ctx.handleTextBlock(v, entering)

		// Inline nodes.
		case *ast.Text:
			ctx.handleText(v, entering)
		case *ast.String:
			ctx.handleString(v, entering)
		case *ast.Emphasis:
			ctx.handleEmphasis(v, entering)
		case *ast.CodeSpan:
			ctx.handleCodeSpan(v, entering)
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.Link:
			ctx.handleLink(v, entering)
		case *ast.Image:
			ctx.handleImage(v, entering)
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.AutoLink:
			ctx.handleAutoLink(v, entering)

		// GFM extension nodes.
		case *east.Table:
			ctx.handleTable(v, entering)
		case *east.TableHeader:
			ctx.handleTableHeader(v, entering)
		case *east.TableRow:
			ctx.handleTableRow(v, entering)
		case *east.TableCell:
			ctx.handleTableCell(v, entering)
		case *east.Strikethrough:
			ctx.handleStrikethrough(v, entering)
		case *east.TaskCheckBox:
			ctx.handleTaskCheckBox(v, entering)
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, err
	}

	return ctx.blocks, nil
}

// ChunkBlocks splits a slice of blocks into chunks of at most maxPerMessage,
// while also ensuring each chunk contains at most one [slack.TableBlock].
// Slack rejects messages with more than one table (error: only_one_table_allowed),
// so this function forces a new chunk before every additional TableBlock.
// If maxPerMessage is <= 0, it defaults to 50.
func ChunkBlocks(blocks []slack.Block, maxPerMessage int) [][]slack.Block {
	if maxPerMessage <= 0 {
		maxPerMessage = 50
	}
	if len(blocks) == 0 {
		return nil
	}

	var chunks [][]slack.Block
	var chunk []slack.Block
	hasTable := false

	for _, b := range blocks {
		_, isTable := b.(*slack.TableBlock)
		if isTable && hasTable {
			// Flush current chunk before starting a new one with this table.
			chunks = append(chunks, chunk)
			chunk = nil
			hasTable = false
		} else if len(chunk) >= maxPerMessage {
			chunks = append(chunks, chunk)
			chunk = nil
			hasTable = false
		}
		chunk = append(chunk, b)
		if isTable {
			hasTable = true
		}
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}
	return chunks
}
