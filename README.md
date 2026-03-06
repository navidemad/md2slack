# md2slack

[![Go Reference](https://pkg.go.dev/badge/github.com/navidemad/md2slack.svg)](https://pkg.go.dev/github.com/navidemad/md2slack)
[![CI](https://github.com/navidemad/md2slack/actions/workflows/ci.yml/badge.svg)](https://github.com/navidemad/md2slack/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/navidemad/md2slack)](https://goreportcard.com/report/github.com/navidemad/md2slack)

> Convert standard Markdown into Slack [Block Kit] blocks.

- **AST-based parsing** — uses [goldmark] with GFM extensions for correct handling of nested/complex markdown
- **Native Slack types** — returns `[]slack.Block` from [slack-go/slack], ready for direct API use
- **Rich output** — headers, dividers, images, rich text (bold, italic, strikethrough, code), lists with nested indent, action buttons, blockquotes, code blocks, and tables
- **GFM support** — tables, strikethrough, task checkboxes, autolinks
- **Message chunking** — `ChunkBlocks` splits output for Slack's 50-block-per-message limit

## Install

```bash
go get github.com/navidemad/md2slack
```

Requires **Go 1.25+**.

## Quick start

```go
import (
	"github.com/navidemad/md2slack"
	"github.com/slack-go/slack"
)

blocks, err := md2slack.Convert("# Welcome\n\nHello **world**.\n\n---\n\n![banner](https://example.com/banner.png)")
```

Output blocks (as JSON):

```json
[
  {
    "type": "header",
    "text": { "type": "plain_text", "text": "Welcome" }
  },
  {
    "type": "rich_text",
    "elements": [
      {
        "type": "rich_text_section",
        "elements": [
          { "type": "text", "text": "Hello " },
          { "type": "text", "text": "world", "style": { "bold": true } },
          { "type": "text", "text": "." }
        ]
      }
    ]
  },
  { "type": "divider" },
  {
    "type": "image",
    "image_url": "https://example.com/banner.png",
    "alt_text": "banner"
  }
]
```

### Block type mapping

| Markdown construct | Block Kit block type |
| :--- | :--- |
| `# Heading` through `######` | `header` (plain\_text, max 150 chars; falls back to bold mrkdwn `section` for links or overflow) |
| `---`, `***`, `___` | `divider` |
| `![alt](url)` (standalone) | `image` |
| `[text](url)` (standalone) | `actions` (button element) |
| `> quote` | `rich_text` (`rich_text_quote`) |
| Fenced code blocks (`` ``` ``) | `rich_text` (`rich_text_preformatted`) |
| `1. item` / `- item` | `rich_text` (`rich_text_list` with ordered/bullet style, nested indent) |
| GFM tables | `table` (native TableBlock with rich text cells, per-column alignment and wrapping) |
| Inline text with formatting | `rich_text` (`rich_text_section` with styled elements) |

### Inline formatting

Bold, italic, strikethrough, and inline code are represented as `RichTextSectionTextElement` entries with style flags rather than mrkdwn strings:

```go
blocks, _ := md2slack.Convert("**bold** and _italic_ and ~~strike~~ and `code`")
```

Links become `RichTextSectionLinkElement` with the URL and display text.

### Lists

Lists are emitted as `rich_text` blocks with `rich_text_list` elements. Nested sub-lists use Slack's `indent` field:

```go
blocks, _ := md2slack.Convert("1. First\n   - Sub-item A\n   - Sub-item B\n2. Second")
```

```json
[
  {
    "type": "rich_text",
    "elements": [
      {
        "type": "rich_text_list",
        "style": "ordered",
        "indent": 0,
        "elements": [
          {
            "type": "rich_text_section",
            "elements": [{ "type": "text", "text": "First" }]
          }
        ]
      },
      {
        "type": "rich_text_list",
        "style": "bullet",
        "indent": 1,
        "elements": [
          {
            "type": "rich_text_section",
            "elements": [{ "type": "text", "text": "Sub-item A" }]
          },
          {
            "type": "rich_text_section",
            "elements": [{ "type": "text", "text": "Sub-item B" }]
          }
        ]
      },
      {
        "type": "rich_text_list",
        "style": "ordered",
        "indent": 0,
        "elements": [
          {
            "type": "rich_text_section",
            "elements": [{ "type": "text", "text": "Second" }]
          }
        ]
      }
    ]
  }
]
```

### Standalone links as buttons

A paragraph containing only a markdown link becomes an `actions` block with a clickable button:

```go
blocks, _ := md2slack.Convert("[Click here](https://example.com)")
```

```json
[
  {
    "type": "actions",
    "elements": [
      {
        "type": "button",
        "text": { "type": "plain_text", "text": "Click here" },
        "url": "https://example.com"
      }
    ]
  }
]
```

### Message chunking

Slack limits messages to 50 blocks. Use `ChunkBlocks` to split. It also enforces at most one `TableBlock` per chunk, since Slack rejects messages containing multiple tables.

```go
blocks, _ := md2slack.Convert(longMarkdown)
chunks := md2slack.ChunkBlocks(blocks, 50)
for _, chunk := range chunks {
    // post each chunk as a separate message
}
```

## API

### `Convert(markdown string) ([]slack.Block, error)`

Parses a Markdown string and returns Slack Block Kit blocks. Returns `nil, nil` for empty input.

### `ChunkBlocks(blocks []slack.Block, maxPerMessage int) [][]slack.Block`

Splits a block slice into chunks of at most `maxPerMessage`. Defaults to 50 if `maxPerMessage <= 0`. Each chunk contains at most one `TableBlock` — when a table is encountered, the current chunk is finalized and a new chunk begins after the table.

## CLI example

The `cmd/example` directory contains a CLI tool that reads markdown from stdin or a file and prints the Block Kit JSON:

```bash
echo "## Hello\n\n**bold** and _italic_" | go run ./cmd/example/
```

## Dependencies

| Package | Purpose |
| :--- | :--- |
| [github.com/yuin/goldmark](https://github.com/yuin/goldmark) | GFM-compliant Markdown parser (AST-based) |
| [github.com/slack-go/slack](https://github.com/slack-go/slack) | Canonical Slack Block Kit types |

## Demo

See [DEMO.md](DEMO.md) for a standalone script that posts converted Markdown to a Slack channel.

## Contributing

Fork the repository and open a pull request. Contributions are welcome!

## License

[MIT](LICENSE)

[goldmark]: https://github.com/yuin/goldmark
[slack-go/slack]: https://github.com/slack-go/slack
[Block Kit]: https://api.slack.com/block-kit
