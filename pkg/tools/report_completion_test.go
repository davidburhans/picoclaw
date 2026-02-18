package tools

import (
	"context"
	"testing"
)

func TestReportCompletion_Name(t *testing.T) {
	tool := &ReportCompletionTool{}
	if tool.Name() != "report_completion" {
		t.Errorf("Expected name 'report_completion', got '%s'", tool.Name())
	}
}

func TestReportCompletion_Execute(t *testing.T) {
	tool := &ReportCompletionTool{}
	ctx := context.Background()
	args := map[string]interface{}{
		"summary": "Done everything.",
	}

	result := tool.Execute(ctx, args)
	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Err)
	}
	if !result.Silent {
		t.Error("ReportCompletion should be silent for the LLM turn")
	}
	if result.ForLLM != "Done everything." {
		t.Errorf("Expected summary in ForLLM, got: %s", result.ForLLM)
	}
}
