package stats

import (
	"fmt"
	"log"
)

func logToolCall(pluginName, toolName string, durationMs int64, inputTokens, outputTokens, outputBytes int) {
	log.Printf("[stats] %s/%s: %dms in=%dtk out=%dtk (%s)",
		pluginName, toolName, durationMs, inputTokens, outputTokens,
		humanBytes(outputBytes))
}

func humanBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
