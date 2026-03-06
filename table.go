package md2slack

import (
	"fmt"

	"github.com/slack-go/slack"
	east "github.com/yuin/goldmark/extension/ast"
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

		ctx.emitBlock(ctx.tableState.renderTableBlock(ctx))
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
			// Empty cell: provide a minimal RichTextSection with a single space
			// so Slack API receives non-empty text (it rejects "text":"").
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

// renderTableBlock builds a *slack.TableBlock from the accumulated table data.
func (ts *tableState) renderTableBlock(ctx *renderContext) *slack.TableBlock {
	ctx.actionCounter++
	blockID := fmt.Sprintf("table-%d", ctx.actionCounter)
	tb := slack.NewTableBlock(blockID)

	// Add header row.
	if len(ts.headerRow) > 0 {
		tb.AddRow(ts.headerRow...)
	}

	// Add data rows.
	for _, row := range ts.dataRows {
		tb.AddRow(row...)
	}

	// Determine column count.
	numCols := 0
	if len(ts.headerRow) > numCols {
		numCols = len(ts.headerRow)
	}
	for _, row := range ts.dataRows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	// Build column settings with alignment and wrapping.
	settings := make([]slack.ColumnSetting, numCols)
	for i := range settings {
		settings[i].IsWrapped = true
		if i < len(ts.alignments) {
			switch ts.alignments[i] {
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
	tb.WithColumnSettings(settings...)

	return tb
}
