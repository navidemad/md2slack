// Package md2slack converts standard Markdown into Slack Block Kit blocks
// using proper AST-based parsing via goldmark and canonical Slack types from
// slack-go/slack.
//
// It provides two main functions:
//
//   - [Convert] parses Markdown (including GFM extensions) and returns
//     []slack.Block suitable for use in the "blocks" field of Slack API
//     payloads. Headings become HeaderBlocks, horizontal rules become
//     DividerBlocks, images become ImageBlocks, fenced code becomes
//     RichTextPreformatted, lists become RichTextList, blockquotes become
//     RichTextQuote, tables become TableBlocks with rich text cells,
//     and inline text with formatting becomes RichTextSection elements.
//
//   - [ChunkBlocks] splits a block slice into chunks of at most N blocks,
//     useful for respecting Slack's 50-block-per-message limit. It also
//     ensures each chunk contains at most one TableBlock, since Slack
//     rejects messages with multiple tables.
//
// # Supported Markdown features
//
//   - Headings (# through ######) → HeaderBlock (plain_text, max 150 chars)
//   - Bold (**text**) → RichTextSectionTextStyle{Bold: true}
//   - Italic (*text* / _text_) → RichTextSectionTextStyle{Italic: true}
//   - Bold+Italic (***text***) → Both Bold and Italic
//   - Strikethrough (~~text~~) → RichTextSectionTextStyle{Strike: true}
//   - Inline code (`code`) → RichTextSectionTextStyle{Code: true}
//   - Links ([text](url)) → RichTextSectionLinkElement
//   - Images (![alt](url)) → ImageBlock (standalone) or link (inline)
//   - Fenced code blocks (```) → RichTextPreformatted
//   - Blockquotes (> text) → RichTextQuote
//   - Ordered lists (1. item) → RichTextList (ordered)
//   - Unordered lists (- item) → RichTextList (bullet)
//   - Nested lists → RichTextList with indent levels
//   - GFM tables → TableBlock with rich text cells
//   - Horizontal rules (---) → DividerBlock
//   - Standalone links → ActionBlock with button
//   - Task checkboxes (- [x] item) → checkbox emoji
//   - Reference-style links ([text][ref]) → resolved via goldmark
//
// # Dependencies
//
// This package uses goldmark for Markdown parsing and slack-go/slack for
// canonical Slack Block Kit types.
//
// [Block Kit]: https://api.slack.com/block-kit
package md2slack
