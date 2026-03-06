# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

md2slack is a Go library that converts standard Markdown into Slack Block Kit blocks using goldmark (GFM parser) and slack-go/slack (canonical Block Kit types). One public function: `Convert` returns `[]slack.Block`. A helper `ChunkBlocks` splits block slices for the 50-block-per-message limit and ensures at most one `TableBlock` per chunk (Slack rejects messages with multiple tables). Requires Go 1.25+.

## Commands

- **Run all tests:** `go test ./... -v`
- **Run a single test:** `go test ./... -v -run TestConvert_Basic/heading`
- **Vet:** `go vet ./...`
- **Fuzz:** `go test -fuzz FuzzConvert -fuzztime 30s`
- **Example CLI:** `echo "## Hello\n\n**bold**" | go run ./cmd/example/`

## Architecture

Single flat package `md2slack` with a custom goldmark AST walker that builds `[]slack.Block`.

- `renderer.go` — Public API: `Convert(markdown string) ([]slack.Block, error)` and `ChunkBlocks(blocks []slack.Block, maxPerMessage int) [][]slack.Block`. Creates a goldmark instance with GFM extension, parses markdown to AST, walks the tree dispatching to handler methods on `renderContext`. The `ast.Walk` function visits each node with entering/leaving callbacks.
- `context.go` — `renderContext` struct that tracks all rendering state: output `blocks` accumulator, `inlineElements` for current inline content, `styleStack` for nested bold/italic/strike/code, heading state (`headingBuf`/`headingMrkdwnBuf` builders for plain text and mrkdwn fallback), `blockquoteStack` for nested blockquotes, `listStack` for nested lists, table/link/image state, `actionCounter` for unique IDs. Contains style stack methods (`pushStyle`, `popStyle`, `recomputeStyle`), inline helpers (`addText`, `addLink`, `flushInlineToSection`), and block emission (`emitBlock`).
- `blocks.go` — Block-level AST node handlers: `handleDocument`, `handleHeading` (smart HeaderBlock vs SectionBlock fallback for links or >150 chars), `handleParagraph` (standalone image → ImageBlock, standalone link → ActionBlock, normal → RichTextBlock), `handleBlockquote` (RichTextQuote), `handleFencedCodeBlock`/`handleCodeBlock` (RichTextPreformatted), `handleList`/`handleListItem` (RichTextList with nested indent), `handleThematicBreak` (DividerBlock).
- `inlines.go` — Inline AST node handlers: `handleText` (text with style stack), `handleString`, `handleEmphasis` (level 1=italic, 2=bold), `handleCodeSpan` (Code style), `handleLink` (RichTextSectionLinkElement), `handleImage`, `handleAutoLink`, `handleStrikethrough` (Strike style), `handleTaskCheckBox` (checkbox emoji).
- `table.go` — GFM table handlers: `handleTable`, `handleTableHeader`, `handleTableRow`, `handleTableCell`. Accumulates cells as `*slack.RichTextBlock` in `tableState`, renders as native `slack.TableBlock` with per-column alignment and wrapping. Inline formatting (bold, links, code) is preserved in cells via the standard inline pipeline.
- `doc.go` — Package-level godoc.
- `cmd/example/main.go` — Example CLI that reads markdown from stdin/file, calls Convert and ChunkBlocks, prints JSON.

## Dependencies

- `github.com/yuin/goldmark` — GFM-compliant Markdown parser (AST-based)
- `github.com/slack-go/slack` — Canonical Slack Block Kit types

## Conventions

- Package stays flat — no subpackages unless API surface grows significantly
- Exported names rely on package qualifier: `md2slack.Convert`, not `md2slack.ConvertMarkdownToSlack`
- Uses slack-go types directly — no custom Block Kit types needed
- Tests use table-driven style with descriptive sub-test names and `blockJSON` helper for readable diffs
- The style stack (bold/italic/strike/code) uses OR-merge of all active frames, supporting arbitrary nesting
- Goldmark splits text into multiple `ast.Text` nodes — tests should concatenate elements to verify content
- Handler methods follow the `entering`/`leaving` pattern matching goldmark's AST walk convention
- Block-level code nodes (FencedCodeBlock, CodeBlock) and inline nodes that manage their own children (CodeSpan, Image) return `WalkSkipChildren` on entering
