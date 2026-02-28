package md2slack

import (
	"strings"

	"github.com/slack-go/slack"
)

// renderContext tracks all state during the AST walk.
type renderContext struct {
	source []byte
	blocks []slack.Block

	// Inline accumulator.
	inlineElements []slack.RichTextSectionElement

	// Style stack for nested bold/italic/strike/code.
	styleStack   []slack.RichTextSectionTextStyle
	currentStyle *slack.RichTextSectionTextStyle

	// Heading state.
	inHeading             bool
	headingLevel          int
	headingBuf            strings.Builder
	headingMrkdwnBuf      strings.Builder
	headingHasUnsupported bool // links, images, etc.

	// Blockquote state (stack for nesting).
	blockquoteStack []blockquoteFrame

	// List state.
	listStack []listFrame

	// Table state.
	inTable    bool
	tableState *tableState

	// Link state.
	inLink      bool
	linkURL     string
	linkTextBuf string // accumulates text for the link display text

	// Image state.
	inImage  bool
	imageURL string
	imageAlt string

	// Action ID counter for unique block/action IDs.
	actionCounter int

	// Paragraph state — detect standalone images/links.
	paragraphChildCount int
	paragraphImageURL   string
	paragraphImageAlt   string
	paragraphLinkURL    string
	paragraphLinkText   string
	isStandaloneImage   bool
	isStandaloneLink    bool
}

// listFrame tracks one level of list nesting.
type listFrame struct {
	style       slack.RichTextListElementType // RTEListOrdered or RTEListBullet
	items       []slack.RichTextElement       // accumulated RichTextSection items
	indent      int
	nestedLists []slack.RichTextElement // nested lists collected from children, emitted as siblings
}

// blockquoteFrame tracks one level of blockquote nesting.
type blockquoteFrame struct {
	elements []slack.RichTextSectionElement
}

// inBlockquote returns true if we are inside a blockquote.
func (ctx *renderContext) inBlockquote() bool {
	return len(ctx.blockquoteStack) > 0
}

// pushStyle pushes a new style onto the style stack and recomputes the effective style.
func (ctx *renderContext) pushStyle(s slack.RichTextSectionTextStyle) {
	ctx.styleStack = append(ctx.styleStack, s)
	ctx.recomputeStyle()
}

// popStyle pops the most recent style from the stack and recomputes.
func (ctx *renderContext) popStyle() {
	if len(ctx.styleStack) > 0 {
		ctx.styleStack = ctx.styleStack[:len(ctx.styleStack)-1]
	}
	ctx.recomputeStyle()
}

// recomputeStyle OR-merges all active style stack frames.
func (ctx *renderContext) recomputeStyle() {
	if len(ctx.styleStack) == 0 {
		ctx.currentStyle = nil
		return
	}
	merged := slack.RichTextSectionTextStyle{}
	for _, s := range ctx.styleStack {
		if s.Bold {
			merged.Bold = true
		}
		if s.Italic {
			merged.Italic = true
		}
		if s.Strike {
			merged.Strike = true
		}
		if s.Code {
			merged.Code = true
		}
	}
	ctx.currentStyle = &merged
}

// effectiveStyle returns a copy of the current merged style, or nil if no styles active.
func (ctx *renderContext) effectiveStyle() *slack.RichTextSectionTextStyle {
	if ctx.currentStyle == nil {
		return nil
	}
	s := *ctx.currentStyle
	return &s
}

// addText adds a text element with the current style to the inline accumulator.
func (ctx *renderContext) addText(text string) {
	if text == "" {
		return
	}
	elem := slack.NewRichTextSectionTextElement(text, ctx.effectiveStyle())
	ctx.inlineElements = append(ctx.inlineElements, elem)
}

// addLink adds a link element to the inline accumulator.
func (ctx *renderContext) addLink(url, text string) {
	elem := slack.NewRichTextSectionLinkElement(url, text, ctx.effectiveStyle())
	ctx.inlineElements = append(ctx.inlineElements, elem)
}

// flushInlineToSection wraps current inline elements in a RichTextSection and returns it.
// Clears the inline accumulator.
func (ctx *renderContext) flushInlineToSection() *slack.RichTextSection {
	if len(ctx.inlineElements) == 0 {
		return nil
	}
	sec := slack.NewRichTextSection(ctx.inlineElements...)
	ctx.inlineElements = nil
	return sec
}

// emitBlock appends a block to the output.
func (ctx *renderContext) emitBlock(b slack.Block) {
	ctx.blocks = append(ctx.blocks, b)
}

// inList returns true if we are inside a list.
func (ctx *renderContext) inList() bool {
	return len(ctx.listStack) > 0
}
