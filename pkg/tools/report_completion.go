package tools

import (
	"context"
)

// ReportCompletionTool is a sentinel tool that sub-agents call to signal task completion.
// The ToolLoop intercepts this tool call and stops execution early.
type ReportCompletionTool struct{}

func (t *ReportCompletionTool) Name() string {
	return "report_completion"
}

func (t *ReportCompletionTool) Description() string {
	return "Call this when you have achieved your objective. Pass a summary of what was accomplished to return it to the supervisor."
}

func (t *ReportCompletionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Final summary of what was accomplished.",
			},
		},
		"required": []string{"summary"},
	}
}

func (t *ReportCompletionTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	summary, _ := args["summary"].(string)
	return SilentResult(summary)
}
