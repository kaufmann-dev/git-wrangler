package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/ui"
)

type summaryCount struct {
	label string
	value int
	color string
}

type keyValueRow struct {
	key   string
	value string
}

type tableColumn struct {
	header string
	max    int
}

type statusState string

const (
	statusOK    statusState = "OK"
	statusWarn  statusState = "WARN"
	statusError statusState = "ERROR"
	statusSkip  statusState = "SKIP"
	statusInfo  statusState = "INFO"
)

func renderSummary(a *app, counts ...summaryCount) {
	fmt.Fprint(a.stdout, "Summary:")
	for i, count := range counts {
		if i == 0 {
			fmt.Fprint(a.stdout, " ")
		} else {
			fmt.Fprint(a.stdout, ", ")
		}
		fmt.Fprintf(a.stdout, "%s%d%s %s", count.color, count.value, a.ui.Reset, count.label)
	}
	fmt.Fprintln(a.stdout)
}

func renderStatusLine(a *app, stream io.Writer, state statusState, subject, detail string) {
	theme := a.ui
	if stream == a.stderr {
		theme = ui.New(a.stderr)
	}
	color, symbol := statusStyle(theme, state)
	label := string(state)
	if ui.UnicodeEnabled(stream) && symbol != "" && symbol != label {
		label = symbol + " " + label
	}
	message := subject
	if detail != "" {
		if message != "" {
			message += ": "
		}
		message += detail
	}
	if message == "" {
		fmt.Fprintf(stream, "%s%s%s\n", color, label, theme.Reset)
		return
	}
	fmt.Fprintf(stream, "%s%s%s %s\n", color, label, theme.Reset, message)
}

func statusStyle(theme ui.Theme, state statusState) (string, string) {
	switch state {
	case statusOK:
		return theme.Green, theme.OKSymbol
	case statusWarn:
		return theme.Yellow, theme.WarnSymbol
	case statusError:
		return theme.Red, theme.ErrSymbol
	case statusSkip:
		return theme.Yellow, theme.SkipSymbol
	default:
		return theme.Cyan, theme.InfoSymbol
	}
}

func renderErrorBlock(a *app, subject, output string) {
	renderStatusLine(a, a.stderr, statusError, subject, "")
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return
	}
	for _, line := range strings.Split(output, "\n") {
		fmt.Fprintf(a.stderr, "  %s\n", line)
	}
	fmt.Fprintln(a.stderr)
}

func outputOrError(output string, err error) string {
	if strings.TrimSpace(output) != "" || err == nil {
		return output
	}
	return err.Error()
}

func renderWarning(a *app, message string) {
	renderStatusLine(a, a.stderr, statusWarn, message, "")
}

func renderNotice(a *app, title string, rows []keyValueRow, bodyLines []string) {
	fmt.Fprintln(a.stderr, title)
	renderKeyValuesTo(a.stderr, rows)
	for _, line := range bodyLines {
		fmt.Fprintln(a.stderr, line)
	}
}

func renderRepoHeader(a *app, name string) {
	fmt.Fprintf(a.stdout, "%sRepository:%s %s%s%s\n", a.ui.Muted, a.ui.Reset, a.ui.RepoColor, name, a.ui.Reset)
}

func renderKeyValues(a *app, rows []keyValueRow) {
	renderKeyValuesTo(a.stdout, rows)
}

func renderKeyValuesTo(w io.Writer, rows []keyValueRow) {
	width := 0
	for _, row := range rows {
		if len(row.key) > width {
			width = len(row.key)
		}
	}
	for _, row := range rows {
		fmt.Fprintf(w, "%-*s  %s\n", width, row.key, row.value)
	}
}

func renderTable(a *app, columns []tableColumn, rows [][]string) {
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = visibleWidth(col.header)
	}
	for _, row := range rows {
		for i := range columns {
			if i >= len(row) {
				continue
			}
			cell := row[i]
			width := visibleWidth(cell)
			if columns[i].max > 0 && width > columns[i].max {
				width = columns[i].max
			}
			if width > widths[i] {
				widths[i] = width
			}
		}
	}
	writeTableRow(a.stdout, columns, widths, nil)
	for _, row := range rows {
		writeTableRow(a.stdout, columns, widths, row)
	}
}

func writeTableRow(w io.Writer, columns []tableColumn, widths []int, row []string) {
	for i, col := range columns {
		cell := col.header
		if row != nil && i < len(row) {
			cell = row[i]
			if col.max > 0 {
				cell = truncateToVisibleWidth(cell, col.max, "\033[0m")
			}
		}
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprint(w, cell)
		padding := widths[i] - visibleWidth(cell)
		if padding > 0 {
			fmt.Fprint(w, strings.Repeat(" ", padding))
		}
	}
	fmt.Fprintln(w)
}

func finishProgressBeforeOutput(progress *progress) {
	progress.done()
}
