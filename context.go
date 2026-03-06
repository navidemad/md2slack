package md2slack

import (
	"regexp"
	"strings"

	"github.com/slack-go/slack"
)

// emojiShortcodeRe matches potential Slack emoji shortcodes like :bar_chart:, :+1:, :wave:.
// Slack emoji names start with a letter, digit, or plus sign, followed by letters, digits,
// underscores, hyphens, or plus signs. This regex intentionally over-matches (e.g. pure-digit
// names like :49:); the companion function isValidEmojiName filters out false positives.
var emojiShortcodeRe = regexp.MustCompile(`:([a-z0-9+][a-z0-9_+\-]*):`)

// isValidEmojiName returns true if the name contains at least one ASCII letter (a-z) or
// a plus sign. Pure-digit names like "49" from time strings (19:49:41) are not valid emojis.
func isValidEmojiName(name string) bool {
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || c == '+' {
			return true
		}
	}
	return false
}

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
// Clears the inline accumulator. Emoji shortcodes are resolved before flushing.
func (ctx *renderContext) flushInlineToSection() *slack.RichTextSection {
	if len(ctx.inlineElements) == 0 {
		return nil
	}
	resolved := resolveEmojis(ctx.inlineElements)
	sec := slack.NewRichTextSection(resolved...)
	ctx.inlineElements = nil
	return sec
}

// stylesEqual returns true if two text styles are equivalent.
func stylesEqual(a, b *slack.RichTextSectionTextStyle) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Bold == b.Bold && a.Italic == b.Italic && a.Strike == b.Strike && a.Code == b.Code
}

// resolveEmojis post-processes a slice of RichTextSectionElements to find and convert
// emoji shortcodes (e.g. :bar_chart:) into RichTextSectionEmojiElement objects.
//
// Goldmark's emphasis parser can split text at underscores, fragmenting emoji shortcodes
// like ":bar_chart:" into separate text elements (":bar_" and "chart:"). This function
// first merges adjacent text elements with the same style, then scans the merged text
// for emoji shortcodes and emits proper emoji elements.
func resolveEmojis(elements []slack.RichTextSectionElement) []slack.RichTextSectionElement {
	if len(elements) == 0 {
		return elements
	}

	// Step 1: Merge adjacent text elements with the same style.
	// This reassembles text that goldmark split at underscore boundaries.
	merged := mergeAdjacentText(elements)

	// Step 2: Scan merged text elements for emoji shortcodes and split them
	// into text + emoji elements.
	var result []slack.RichTextSectionElement
	for _, elem := range merged {
		te, ok := elem.(*slack.RichTextSectionTextElement)
		if !ok || (te.Style != nil && te.Style.Code) {
			// Non-text elements and code-styled text are passed through as-is.
			// Inline code (backticks) should render :fire: literally, not as an emoji.
			result = append(result, elem)
			continue
		}

		matches := emojiShortcodeRe.FindAllStringIndex(te.Text, -1)
		if matches == nil {
			result = append(result, elem)
			continue
		}

		cursor := 0
		for _, loc := range matches {
			name := te.Text[loc[0]+1 : loc[1]-1]
			if !isValidEmojiName(name) {
				// Not a valid emoji (e.g. pure digits from time strings like 19:49:41).
				// Leave the text as-is and skip this match.
				continue
			}
			if loc[0] > cursor {
				result = append(result, slack.NewRichTextSectionTextElement(te.Text[cursor:loc[0]], te.Style))
			}
			result = append(result, slack.NewRichTextSectionEmojiElement(name, 0, te.Style))
			cursor = loc[1]
		}
		if cursor < len(te.Text) {
			result = append(result, slack.NewRichTextSectionTextElement(te.Text[cursor:], te.Style))
		}
	}

	return result
}

// mergeAdjacentText combines consecutive RichTextSectionTextElement entries
// that share the same style into single elements. This is necessary because
// goldmark's emphasis parser splits text at underscore boundaries, which
// fragments emoji shortcodes like ":bar_chart:" across multiple elements.
func mergeAdjacentText(elements []slack.RichTextSectionElement) []slack.RichTextSectionElement {
	if len(elements) <= 1 {
		return elements
	}

	result := make([]slack.RichTextSectionElement, 0, len(elements))
	for _, elem := range elements {
		te, ok := elem.(*slack.RichTextSectionTextElement)
		if !ok || len(result) == 0 {
			result = append(result, elem)
			continue
		}

		prev, prevOk := result[len(result)-1].(*slack.RichTextSectionTextElement)
		if prevOk && stylesEqual(prev.Style, te.Style) {
			// Merge into the previous element.
			result[len(result)-1] = slack.NewRichTextSectionTextElement(prev.Text+te.Text, prev.Style)
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// emitBlock appends a block to the output.
func (ctx *renderContext) emitBlock(b slack.Block) {
	if b == nil {
		return
	}
	ctx.blocks = append(ctx.blocks, b)
}

// inList returns true if we are inside a list.
func (ctx *renderContext) inList() bool {
	return len(ctx.listStack) > 0
}
