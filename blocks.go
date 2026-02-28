package md2slack

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/slack-go/slack"
	"github.com/yuin/goldmark/ast"
)

// handleDocument processes the root Document node.
func (ctx *renderContext) handleDocument(_ *ast.Document, entering bool) {
	// No-op on entering. On leaving, nothing to flush since
	// all blocks should have been emitted by their own handlers.
}

// handleHeading processes an ast.Heading node.
func (ctx *renderContext) handleHeading(n *ast.Heading, entering bool) {
	if entering {
		ctx.inHeading = true
		ctx.headingLevel = n.Level
		ctx.headingBuf.Reset()
		ctx.headingMrkdwnBuf.Reset()
		ctx.headingHasUnsupported = false
		ctx.inlineElements = nil
	} else {
		ctx.inHeading = false
		text := strings.TrimSpace(ctx.headingBuf.String())

		// Slack header blocks: plain_text only, max 150 chars, no links/images.
		if !ctx.headingHasUnsupported && utf8.RuneCountInString(text) <= 150 {
			ctx.emitBlock(slack.NewHeaderBlock(
				slack.NewTextBlockObject(slack.PlainTextType, text, false, false),
			))
		} else {
			// Fallback: section block with bold mrkdwn that preserves links.
			mrkdwn := "*" + strings.TrimSpace(ctx.headingMrkdwnBuf.String()) + "*"
			ctx.emitBlock(slack.NewSectionBlock(
				slack.NewTextBlockObject(slack.MarkdownType, mrkdwn, false, false),
				nil, nil,
			))
		}
		ctx.headingBuf.Reset()
		ctx.headingMrkdwnBuf.Reset()
		ctx.inlineElements = nil
	}
}

// handleParagraph processes an ast.Paragraph node.
func (ctx *renderContext) handleParagraph(n *ast.Paragraph, entering bool) {
	if entering {
		ctx.inlineElements = nil
		ctx.paragraphChildCount = 0
		ctx.isStandaloneImage = false
		ctx.isStandaloneLink = false
		ctx.paragraphImageURL = ""
		ctx.paragraphImageAlt = ""
		ctx.paragraphLinkURL = ""
		ctx.paragraphLinkText = ""

		// Count direct children to detect standalone image/link.
		count := 0
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			count++
		}
		ctx.paragraphChildCount = count

		// Check if single child is an image.
		if count == 1 {
			if img, ok := n.FirstChild().(*ast.Image); ok {
				ctx.isStandaloneImage = true
				ctx.paragraphImageURL = string(img.Destination)
				// Collect alt text from children.
				var alt string
				for c := img.FirstChild(); c != nil; c = c.NextSibling() {
					if t, ok := c.(*ast.Text); ok {
						alt += string(t.Value(ctx.source))
					}
				}
				ctx.paragraphImageAlt = alt
			}
		}

		// Check if single child is a link (not an image).
		if count == 1 {
			if link, ok := n.FirstChild().(*ast.Link); ok {
				ctx.isStandaloneLink = true
				ctx.paragraphLinkURL = string(link.Destination)
				// Collect text from children.
				var text string
				for c := link.FirstChild(); c != nil; c = c.NextSibling() {
					switch v := c.(type) {
					case *ast.Text:
						text += string(v.Value(ctx.source))
					case *ast.CodeSpan:
						for gc := v.FirstChild(); gc != nil; gc = gc.NextSibling() {
							if t, ok := gc.(*ast.Text); ok {
								text += string(t.Value(ctx.source))
							}
						}
					}
				}
				ctx.paragraphLinkText = text
			}
		}

	} else {
		// Leaving paragraph.
		if ctx.inHeading {
			return
		}

		// Standalone image → ImageBlock.
		if ctx.isStandaloneImage {
			alt := ctx.paragraphImageAlt
			if alt == "" {
				alt = " "
			}
			ctx.emitBlock(slack.NewImageBlock(
				ctx.paragraphImageURL, alt, "", nil,
			))
			ctx.inlineElements = nil
			return
		}

		// Standalone link → ActionBlock with button.
		if ctx.isStandaloneLink {
			actionID := fmt.Sprintf("md2slack-link-%d", ctx.actionCounter)
			blockID := fmt.Sprintf("md2slack-action-%d", ctx.actionCounter)
			ctx.actionCounter++
			btn := slack.NewButtonBlockElement(
				actionID, ctx.paragraphLinkURL,
				slack.NewTextBlockObject(slack.PlainTextType, ctx.paragraphLinkText, false, false),
			)
			btn.URL = ctx.paragraphLinkURL
			ctx.emitBlock(slack.NewActionBlock(blockID, btn))
			ctx.inlineElements = nil
			return
		}

		// Inside a list item → don't emit a block, let the list handler deal with it.
		if ctx.inList() {
			return
		}

		// Inside a blockquote → accumulate, let the blockquote handler deal with it.
		if ctx.inBlockquote() {
			frame := &ctx.blockquoteStack[len(ctx.blockquoteStack)-1]
			if len(frame.elements) > 0 {
				// Add a newline separator between blockquote paragraphs.
				frame.elements = append(frame.elements,
					slack.NewRichTextSectionTextElement("\n", nil))
			}
			frame.elements = append(frame.elements, ctx.inlineElements...)
			ctx.inlineElements = nil
			return
		}

		// Normal paragraph → section block with mrkdwn.
		if len(ctx.inlineElements) > 0 {
			sec := slack.NewRichTextSection(ctx.inlineElements...)
			ctx.emitBlock(slack.NewRichTextBlock("", sec))
			ctx.inlineElements = nil
		}
	}
}

// handleBlockquote processes an ast.Blockquote node.
func (ctx *renderContext) handleBlockquote(_ *ast.Blockquote, entering bool) {
	if entering {
		ctx.blockquoteStack = append(ctx.blockquoteStack, blockquoteFrame{})
	} else {
		if len(ctx.blockquoteStack) == 0 {
			return
		}
		frame := ctx.blockquoteStack[len(ctx.blockquoteStack)-1]
		ctx.blockquoteStack = ctx.blockquoteStack[:len(ctx.blockquoteStack)-1]

		if len(frame.elements) == 0 {
			return
		}

		// Nested blockquote: flatten into parent (Slack has no nested quote support).
		if len(ctx.blockquoteStack) > 0 {
			parent := &ctx.blockquoteStack[len(ctx.blockquoteStack)-1]
			if len(parent.elements) > 0 {
				parent.elements = append(parent.elements,
					slack.NewRichTextSectionTextElement("\n", nil))
			}
			parent.elements = append(parent.elements, frame.elements...)
			return
		}

		// Top-level blockquote: emit as RichTextBlock with RichTextQuote.
		quote := &slack.RichTextQuote{
			Type:     slack.RTEQuote,
			Elements: frame.elements,
		}
		ctx.emitBlock(slack.NewRichTextBlock("", quote))
	}
}

// handleFencedCodeBlock processes an ast.FencedCodeBlock node.
func (ctx *renderContext) handleFencedCodeBlock(n *ast.FencedCodeBlock, entering bool) {
	if !entering {
		return
	}

	// Extract all lines from the code block.
	var buf strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(ctx.source))
	}

	text := buf.String()
	// Trim trailing newline that goldmark includes.
	text = strings.TrimRight(text, "\n")

	pre := &slack.RichTextPreformatted{
		RichTextSection: slack.RichTextSection{
			Type: slack.RTEPreformatted,
			Elements: []slack.RichTextSectionElement{
				slack.NewRichTextSectionTextElement(text, nil),
			},
		},
	}
	ctx.emitBlock(slack.NewRichTextBlock("", pre))
}

// handleCodeBlock processes an ast.CodeBlock (indented code block).
func (ctx *renderContext) handleCodeBlock(n *ast.CodeBlock, entering bool) {
	if !entering {
		return
	}

	var buf strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(ctx.source))
	}

	text := strings.TrimRight(buf.String(), "\n")

	pre := &slack.RichTextPreformatted{
		RichTextSection: slack.RichTextSection{
			Type: slack.RTEPreformatted,
			Elements: []slack.RichTextSectionElement{
				slack.NewRichTextSectionTextElement(text, nil),
			},
		},
	}
	ctx.emitBlock(slack.NewRichTextBlock("", pre))
}

// handleList processes an ast.List node.
func (ctx *renderContext) handleList(n *ast.List, entering bool) {
	if entering {
		style := slack.RTEListBullet
		if n.IsOrdered() {
			style = slack.RTEListOrdered
		}
		indent := len(ctx.listStack)

		// Before pushing a nested list, flush the current list item's
		// inline elements so they are not destroyed by the nested list's
		// handleListItem entering calls.
		if indent > 0 && len(ctx.inlineElements) > 0 {
			sec := ctx.flushInlineToSection()
			if sec != nil {
				frame := &ctx.listStack[len(ctx.listStack)-1]
				frame.items = append(frame.items, sec)
			}
		}

		ctx.listStack = append(ctx.listStack, listFrame{
			style:  style,
			indent: indent,
		})
	} else {
		if len(ctx.listStack) == 0 {
			return
		}
		frame := ctx.listStack[len(ctx.listStack)-1]
		ctx.listStack = ctx.listStack[:len(ctx.listStack)-1]

		if len(ctx.listStack) == 0 {
			// Top-level list: emit as a RichTextBlock.
			// Collect the list itself (if non-empty) plus any nested lists
			// as sibling elements in the same RichTextBlock.
			var elements []slack.RichTextElement
			if len(frame.items) > 0 {
				elements = append(elements, slack.NewRichTextList(frame.style, frame.indent, frame.items...))
			}
			elements = append(elements, frame.nestedLists...)
			if len(elements) > 0 {
				ctx.emitBlock(slack.NewRichTextBlock("", elements...))
			}
		} else {
			// Nested list: store in parent frame's nestedLists rather than
			// embedding as a child element (Slack rejects nested rich_text_list
			// inside rich_text_list elements). Also propagate any deeply
			// nested lists upward to maintain sibling ordering.
			parentFrame := &ctx.listStack[len(ctx.listStack)-1]
			if len(frame.items) > 0 {
				parentFrame.nestedLists = append(parentFrame.nestedLists, slack.NewRichTextList(frame.style, frame.indent, frame.items...))
			}
			parentFrame.nestedLists = append(parentFrame.nestedLists, frame.nestedLists...)
		}
	}
}

// handleListItem processes an ast.ListItem node.
func (ctx *renderContext) handleListItem(_ *ast.ListItem, entering bool) {
	if entering {
		ctx.inlineElements = nil
	} else {
		if len(ctx.listStack) == 0 {
			return
		}
		sec := ctx.flushInlineToSection()
		if sec == nil {
			// Don't emit empty sections — this happens when a list item
			// contains only a nested list and no text of its own.
			return
		}

		frame := &ctx.listStack[len(ctx.listStack)-1]
		frame.items = append(frame.items, sec)
	}
}

// handleThematicBreak processes an ast.ThematicBreak node.
func (ctx *renderContext) handleThematicBreak(_ *ast.ThematicBreak, entering bool) {
	if entering {
		ctx.emitBlock(slack.NewDividerBlock())
	}
}

// handleHTMLBlock is a no-op — we skip raw HTML blocks.
func (ctx *renderContext) handleHTMLBlock(_ *ast.HTMLBlock, _ bool) {
}

// handleTextBlock processes an ast.TextBlock node.
func (ctx *renderContext) handleTextBlock(_ *ast.TextBlock, _ bool) {
}
