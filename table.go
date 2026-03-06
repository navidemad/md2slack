package md2slack

import (
	"fmt"

	"github.com/slack-go/slack"
	east "github.com/yuin/goldmark/extension/ast"
)

const (
	// Slack TableBlock API limits.
	// https://docs.slack.dev/reference/block-kit/blocks/table-block/
	maxTableRows    = 100 // including header row
	maxTableColumns = 20
)

// tableState accumulates table data during AST walking.
type tableState struct {
	alignments []east.Alignment
	headerRow  []*slack.RichTextBlock
	dataRows   [][]*slack.RichTextBlock
	currentRow []*slack.RichTextBlock
	inHeader   bool
}

// handleTable processes a GFM Table node.
func (ctx *renderContext) handleTable(n *east.Table, entering bool) {
	if entering {
		ctx.inTable = true
		ctx.tableState = &tableState{
			alignments: n.Alignments,
		}
	} else {
		ctx.inTable = false
		if ctx.tableState == nil {
			return
		}

		for _, tb := range ctx.tableState.renderTableBlocks(ctx) {
			ctx.emitBlock(tb)
		}
		ctx.tableState = nil
	}
}

// handleTableHeader processes a GFM TableHeader node.
func (ctx *renderContext) handleTableHeader(_ *east.TableHeader, entering bool) {
	if ctx.tableState == nil {
		return
	}
	if entering {
		ctx.tableState.inHeader = true
		ctx.tableState.currentRow = nil
	} else {
		ctx.tableState.headerRow = ctx.tableState.currentRow
		ctx.tableState.currentRow = nil
		ctx.tableState.inHeader = false
	}
}

// handleTableRow processes a GFM TableRow node.
func (ctx *renderContext) handleTableRow(_ *east.TableRow, entering bool) {
	if ctx.tableState == nil {
		return
	}
	if entering {
		ctx.tableState.currentRow = nil
	} else {
		ctx.tableState.dataRows = append(ctx.tableState.dataRows, ctx.tableState.currentRow)
		ctx.tableState.currentRow = nil
	}
}

// handleTableCell processes a GFM TableCell node.
func (ctx *renderContext) handleTableCell(_ *east.TableCell, entering bool) {
	if ctx.tableState == nil {
		return
	}
	if entering {
		// Clear inline accumulator and style stack so cell content starts fresh.
		ctx.inlineElements = nil
		ctx.styleStack = nil
		ctx.currentStyle = nil
	} else {
		// Flush accumulated inline elements into a RichTextBlock cell.
		var elements []slack.RichTextElement
		sec := ctx.flushInlineToSection()
		if sec != nil {
			elements = append(elements, sec)
		}
		if len(elements) == 0 {
			// Empty cell: provide a minimal RichTextSection so Slack API
			// receives "elements": [...] rather than null.
			// Note: Slack rejects empty string text elements, so use a space.
			elements = []slack.RichTextElement{
				slack.NewRichTextSection(
					slack.NewRichTextSectionTextElement(" ", nil),
				),
			}
		}
		cell := slack.NewRichTextBlock("", elements...)
		ctx.tableState.currentRow = append(ctx.tableState.currentRow, cell)
	}
}

// renderTableBlocks builds one or more [slack.TableBlock] from the accumulated
// table data. Slack enforces a maximum of [maxTableRows] rows (including the
// header) and [maxTableColumns] columns per table. When the data exceeds the
// row limit, it is split across multiple TableBlocks, each carrying the same
// header. Columns beyond [maxTableColumns] are silently truncated.
func (ts *tableState) renderTableBlocks(ctx *renderContext) []*slack.TableBlock {
	header := truncateRow(ts.headerRow, maxTableColumns)
	alignments := ts.alignments
	if len(alignments) > maxTableColumns {
		alignments = alignments[:maxTableColumns]
	}

	// Maximum data rows per table: total limit minus 1 for the header (if present).
	maxDataRows := maxTableRows
	if len(header) > 0 {
		maxDataRows = maxTableRows - 1
	}

	var tables []*slack.TableBlock
	for start := 0; start < len(ts.dataRows) || len(tables) == 0; {
		end := start + maxDataRows
		if end > len(ts.dataRows) {
			end = len(ts.dataRows)
		}
		chunk := ts.dataRows[start:end]

		ctx.actionCounter++
		blockID := fmt.Sprintf("table-%d", ctx.actionCounter)
		tb := slack.NewTableBlock(blockID)

		if len(header) > 0 {
			tb.AddRow(header...)
		}
		for _, row := range chunk {
			tb.AddRow(truncateRow(row, maxTableColumns)...)
		}

		tb.WithColumnSettings(buildColumnSettings(header, chunk, alignments)...)
		tables = append(tables, tb)
		start = end
	}

	return tables
}

// truncateRow returns the row trimmed to at most maxCols cells.
func truncateRow(row []*slack.RichTextBlock, maxCols int) []*slack.RichTextBlock {
	if len(row) <= maxCols {
		return row
	}
	return row[:maxCols]
}

// buildColumnSettings creates column settings based on the actual column count
// (capped at maxTableColumns) from the header and data rows.
func buildColumnSettings(header []*slack.RichTextBlock, dataRows [][]*slack.RichTextBlock, alignments []east.Alignment) []slack.ColumnSetting {
	numCols := len(header)
	for _, row := range dataRows {
		n := len(row)
		if n > maxTableColumns {
			n = maxTableColumns
		}
		if n > numCols {
			numCols = n
		}
	}
	if numCols > maxTableColumns {
		numCols = maxTableColumns
	}

	settings := make([]slack.ColumnSetting, numCols)
	for i := range settings {
		settings[i].IsWrapped = true
		if i < len(alignments) {
			switch alignments[i] {
			case east.AlignRight:
				settings[i].Align = slack.ColumnAlignmentRight
			case east.AlignCenter:
				settings[i].Align = slack.ColumnAlignmentCenter
			default:
				settings[i].Align = slack.ColumnAlignmentLeft
			}
		} else {
			settings[i].Align = slack.ColumnAlignmentLeft
		}
	}
	return settings
}
