package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/LeGambiArt/wtmcp/internal/stats"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "View tool usage statistics",
	Long: `View accumulated tool usage statistics from the last server session.

Reads stats.json from the cache directory. The server does not need
to be running.`,
	RunE: runStats,
}

func init() {
	statsCmd.Flags().Bool("schemas", false, "Include tool schema token costs")
	statsCmd.Flags().Bool("resources", false, "Include resource read stats")
	statsCmd.Flags().StringP("sort", "s", "calls", "Sort by: calls, tokens, errors, name")
	statsCmd.Flags().BoolP("plain", "p", false, "Plain text output (no colors or borders)")
	statsCmd.Flags().Bool("json", false, "Raw JSON output")
}

func runStats(cmd *cobra.Command, _ []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	plain, _ := cmd.Flags().GetBool("plain")
	showSchemas, _ := cmd.Flags().GetBool("schemas")
	showResources, _ := cmd.Flags().GetBool("resources")
	sortBy, _ := cmd.Flags().GetString("sort")

	snap, err := loadSnapshot()
	if err != nil {
		return err
	}
	if snap == nil {
		fmt.Println("No stats collected yet.")
		return nil
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(snap, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Build sorted tool summaries.
	summaries := make([]stats.ToolSummary, 0, len(snap.Aggregates))
	for _, ts := range snap.Aggregates {
		summaries = append(summaries, ts)
	}
	sortSummaries(summaries, sortBy)

	if plain {
		printStatsPlain(snap, summaries, showSchemas, showResources)
	} else {
		printStatsTable(snap, summaries, showSchemas, showResources)
	}
	return nil
}

func loadSnapshot() (*stats.Snapshot, error) {
	result, err := getDiscoveryResult()
	if err != nil {
		return nil, err
	}

	statsPath := filepath.Join(result.Config.Cache.Dir, "stats.json")
	data, err := os.ReadFile(statsPath) //nolint:gosec // path from config
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil // nil signals "no file"
		}
		return nil, fmt.Errorf("read stats: %w", err)
	}

	var snap stats.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse stats: %w", err)
	}
	return &snap, nil
}

func sortSummaries(s []stats.ToolSummary, by string) {
	switch by {
	case "tokens":
		sort.Slice(s, func(i, j int) bool {
			ti := s[i].TotalInputTokens + s[i].TotalOutputTokens
			tj := s[j].TotalInputTokens + s[j].TotalOutputTokens
			return ti > tj
		})
	case "errors":
		sort.Slice(s, func(i, j int) bool {
			return s[i].ErrorCount > s[j].ErrorCount
		})
	case "name":
		sort.Slice(s, func(i, j int) bool {
			return s[i].ToolName < s[j].ToolName
		})
	default: // "calls"
		sort.Slice(s, func(i, j int) bool {
			return s[i].CallCount > s[j].CallCount
		})
	}
}

func printStatsPlain(snap *stats.Snapshot, summaries []stats.ToolSummary, showSchemas, showResources bool) {
	fmt.Printf("# tokenizer: %s\n", snap.Tokenizer)

	var totalCalls, totalErrors, totalIn, totalOut int
	for _, ts := range summaries {
		fmt.Printf("%s\t%s\t%d\t%d\t%d\t%d\t%d\n",
			ts.PluginName, ts.ToolName, ts.CallCount, ts.ErrorCount,
			ts.TotalInputTokens, ts.TotalOutputTokens, ts.AvgDurationMs)
		totalCalls += ts.CallCount
		totalErrors += ts.ErrorCount
		totalIn += ts.TotalInputTokens
		totalOut += ts.TotalOutputTokens
	}
	fmt.Printf("# totals: %d calls, %d errors, %d input tokens, %d output tokens\n",
		totalCalls, totalErrors, totalIn, totalOut)

	if showSchemas {
		fmt.Println()
		for name, se := range snap.Schemas {
			fmt.Printf("schema\t%s\t%s\t%d\n", se.PluginName, name, se.TotalTokens)
		}
	}

	if showResources {
		fmt.Println()
		for _, re := range snap.Resources {
			fmt.Printf("resource\t%s\t%s\t%s\t%d\t%d\t%d\n",
				re.PluginName, re.URI, re.ResourceType,
				re.ContentBytes, re.ContentTokens, re.ReadCount)
		}
	}
}

func printStatsTable(snap *stats.Snapshot, summaries []stats.ToolSummary, showSchemas, showResources bool) {
	w, _, _ := term.GetSize(os.Stdout.Fd())
	if w <= 0 {
		w = 100
	}

	fmt.Printf("Tool Usage Stats (tokenizer: %s)\n\n", snap.Tokenizer)

	// Main calls table.
	var totalCalls, totalErrors, totalIn, totalOut int
	rows := make([][]string, 0, len(summaries))
	for _, ts := range summaries {
		rows = append(rows, []string{
			ts.PluginName,
			ts.ToolName,
			strconv.Itoa(ts.CallCount),
			strconv.Itoa(ts.ErrorCount),
			strconv.Itoa(ts.TotalInputTokens),
			strconv.Itoa(ts.TotalOutputTokens),
			strconv.FormatInt(ts.AvgDurationMs, 10),
		})
		totalCalls += ts.CallCount
		totalErrors += ts.ErrorCount
		totalIn += ts.TotalInputTokens
		totalOut += ts.TotalOutputTokens
	}

	if len(rows) > 0 {
		t := table.New().
			Width(w).
			Border(lipgloss.RoundedBorder()).
			BorderStyle(borderStyle).
			StyleFunc(func(row, _ int) lipgloss.Style {
				if row == table.HeaderRow {
					return headerStyle
				}
				return lipgloss.NewStyle()
			}).
			Headers("Plugin", "Tool", "Calls", "Errors", "In Tk", "Out Tk", "Avg ms").
			Rows(rows...)
		fmt.Println(t)
	}

	fmt.Printf("\nTotals: %d calls, %d errors, %d input tokens, %d output tokens\n",
		totalCalls, totalErrors, totalIn, totalOut)

	// Schema overhead table.
	if showSchemas && len(snap.Schemas) > 0 {
		fmt.Println("\nSchema Overhead")

		type pluginSchema struct {
			name   string
			tools  int
			tokens int
		}
		byPlugin := make(map[string]*pluginSchema)
		totalSchemaTokens := 0
		for _, se := range snap.Schemas {
			ps, ok := byPlugin[se.PluginName]
			if !ok {
				ps = &pluginSchema{name: se.PluginName}
				byPlugin[se.PluginName] = ps
			}
			ps.tools++
			ps.tokens += se.TotalTokens
			totalSchemaTokens += se.TotalTokens
		}

		schemaRows := make([][]string, 0, len(byPlugin)+1)
		for _, ps := range byPlugin {
			schemaRows = append(schemaRows, []string{
				ps.name,
				strconv.Itoa(ps.tools),
				strconv.Itoa(ps.tokens),
			})
		}
		sort.Slice(schemaRows, func(i, j int) bool {
			return schemaRows[i][0] < schemaRows[j][0]
		})
		schemaRows = append(schemaRows, []string{
			"Total",
			strconv.Itoa(len(snap.Schemas)),
			strconv.Itoa(totalSchemaTokens),
		})

		st := table.New().
			Width(min(w, 50)).
			Border(lipgloss.RoundedBorder()).
			BorderStyle(borderStyle).
			StyleFunc(func(row, _ int) lipgloss.Style {
				if row == table.HeaderRow {
					return headerStyle
				}
				return lipgloss.NewStyle()
			}).
			Headers("Plugin", "Tools", "Tokens").
			Rows(schemaRows...)
		fmt.Println(st)
	}

	// Resource reads table.
	if showResources && len(snap.Resources) > 0 {
		fmt.Println("\nResource Reads")

		resRows := make([][]string, 0, len(snap.Resources))
		for _, re := range snap.Resources {
			resRows = append(resRows, []string{
				re.URI,
				re.PluginName,
				re.ResourceType,
				strconv.Itoa(re.ContentBytes),
				strconv.Itoa(re.ContentTokens),
				strconv.Itoa(re.ReadCount),
			})
		}
		sort.Slice(resRows, func(i, j int) bool {
			return resRows[i][0] < resRows[j][0]
		})

		rt := table.New().
			Width(w).
			Border(lipgloss.RoundedBorder()).
			BorderStyle(borderStyle).
			StyleFunc(func(row, _ int) lipgloss.Style {
				if row == table.HeaderRow {
					return headerStyle
				}
				return lipgloss.NewStyle()
			}).
			Headers("URI", "Plugin", "Type", "Bytes", "Tokens", "Reads").
			Rows(resRows...)
		fmt.Println(rt)
	}
}
