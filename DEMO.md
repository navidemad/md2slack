# Demo: posting to Slack

This guide walks you through using the `cmd/demo/` program to convert Markdown to Slack Block Kit blocks and post the result to a Slack channel.

## Prerequisites

| What | How to get it |
|------|---------------|
| **Go 1.25+** | Install from <https://go.dev/dl/> — run `go version` to verify |
| **Slack Bot Token** | Create an app at <https://api.slack.com/apps>, add the `chat:write` bot scope, install to your workspace, copy the `xoxb-…` token |
| **Channel ID** | In Slack, right-click a channel → *View channel details* → copy the ID at the bottom (e.g. `C0123456789`). Invite the bot to the channel |
| **Claude Code** *(optional)* | Install from <https://docs.claude.com/en/docs/setup> — only needed for the Claude-generated example |

## Step 1 — Set environment variables

Export your Slack credentials:

```bash
export SLACK_TOKEN="xoxb-your-token"
export SLACK_CHANNEL="C0123456789"
```

## Step 2 — Run the demo

The demo program lives at `cmd/demo/` in this repository. It supports three input modes:

**Post the built-in sample** (exercises headings, bold, italic, strikethrough, inline code, code blocks, links, images, lists, blockquotes, tables, and horizontal rules):

```bash
go run ./cmd/demo/ --default-input
```

**Post from a Markdown file:**

```bash
go run ./cmd/demo/ --file-input /path/to/your/file.md
```

**Pipe from stdin:**

```bash
echo "# Hello from md2slack" | go run ./cmd/demo/
```

The program converts Markdown to Block Kit blocks via `md2slack.Convert`, splits them into chunks of 50 (Slack's per-message limit) via `md2slack.ChunkBlocks`, and posts each chunk. If there are multiple chunks, subsequent chunks are automatically threaded under the first message.

## Step 3 — Reply into an existing thread

Use `--thread-ts` (or `--ts`) to post as a reply in an existing thread.

Right-click a message in Slack, select **Copy link**, and pass the URL directly:

```bash
go run ./cmd/demo/ --default-input --thread-ts "https://yourworkspace.slack.com/archives/C0123456789/p1234567890123456"
```

A raw timestamp also works:

```bash
go run ./cmd/demo/ --default-input --thread-ts 1234567890.123456
```

## Flag reference

| Flag | Description |
|------|-------------|
| `--file-input <path>` | Read Markdown from a file |
| `--default-input` | Use the built-in sample Markdown |
| `--thread-ts <ts>` | Reply into a thread (timestamp or Slack message URL) |
| `--ts <ts>` | Alias for `--thread-ts` |

`--file-input` and `--default-input` are mutually exclusive. When neither is given, input is read from stdin.

## Optional: generate Markdown with Claude Code

Use Claude Code to produce Markdown that exercises all the formatting features md2slack supports — headings, bold, italic, strikethrough, inline code, code blocks, links, images, ordered and unordered lists, blockquotes, tables, horizontal rules, and reference-style links:

```bash
claude -p \
  'Print a detailed Markdown document (at least 80 lines) about a fictional
   Go CLI tool called "taskrunner" that executes build pipelines. Use every
   one of these Markdown features with real content:

   - An h1 title, an h2 section heading, and an h3 sub-heading
   - **bold**, _italic_, and ~~strikethrough~~ text inline within sentences
   - `inline code` for function names, commands, and variables
   - A fenced code block with a Go language tag containing a real code sample
   - A fenced code block with a bash language tag showing CLI usage
   - An inline [named link](https://example.com) within a paragraph
   - A standalone link on its own line: [Link text](url)
   - An image on its own line: ![alt text](https://placehold.co/600x200.png)
   - A bulleted list with at least 3 items
   - A numbered list with at least 3 items
   - A blockquote (> ) with at least two lines
   - A Markdown table with a header row, separator, and at least 3 data rows
   - A horizontal rule (---) separating two sections
   - At least two reference-style links using [text][ref] syntax with
     [ref]: url definitions at the bottom of the document

   Output only raw Markdown.' \
  > /tmp/claude_output.md
```

Then post it:

```bash
go run ./cmd/demo/ --file-input /tmp/claude_output.md
```

Or combine generation and posting in one pipeline:

```bash
claude -p "Summarize the latest changes in this repo." \
  --output-format stream-json 2>/dev/null \
  | jq -r 'select(.type == "result") | .result' \
  | go run ./cmd/demo/
```
