package tools

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/family"
)

type AssignChoreTool struct {
	choreManager    *family.ChoreManager
	workspaceName   string
	canManageFamily bool
}

func NewAssignChoreTool(familyPath, workspaceName string, canManageFamily bool) *AssignChoreTool {
	return &AssignChoreTool{
		choreManager:    family.NewChoreManager(familyPath),
		workspaceName:   workspaceName,
		canManageFamily: canManageFamily,
	}
}

func (t *AssignChoreTool) Name() string {
	return "assign_chore"
}

func (t *AssignChoreTool) Description() string {
	return "Assign a chore to a family member (parent only)"
}

func (t *AssignChoreTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Name of the family member to assign the chore to",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Title of the chore",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Description of the chore (optional)",
			},
			"due_date": map[string]interface{}{
				"type":        "string",
				"description": "Due date for the chore (optional)",
			},
			"points": map[string]interface{}{
				"type":        "integer",
				"description": "Points for completing the chore (default 10)",
			},
		},
		"required": []string{"to", "title"},
	}
}

func (t *AssignChoreTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if !t.canManageFamily {
		return ErrorResult("only parents can assign chores")
	}

	to, _ := args["to"].(string)
	title, _ := args["title"].(string)
	description, _ := args["description"].(string)
	dueDate, _ := args["due_date"].(string)

	points := 10
	if p, ok := args["points"].(float64); ok {
		points = int(p)
	}

	if to == "" {
		return ErrorResult("to is required")
	}
	if title == "" {
		return ErrorResult("title is required")
	}

	chore, err := t.choreManager.AssignChore(to, title, description, dueDate, points, t.workspaceName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to assign chore: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Assigned chore %q (ID: %s) to %s (points: %d)", title, chore.ID, to, points))
}

type CompleteChoreTool struct {
	choreManager  *family.ChoreManager
	workspaceName string
}

func NewCompleteChoreTool(familyPath, workspaceName string) *CompleteChoreTool {
	return &CompleteChoreTool{
		choreManager:  family.NewChoreManager(familyPath),
		workspaceName: workspaceName,
	}
}

func (t *CompleteChoreTool) Name() string {
	return "complete_chore"
}

func (t *CompleteChoreTool) Description() string {
	return "Mark a chore as completed"
}

func (t *CompleteChoreTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"chore_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the chore to complete",
			},
		},
		"required": []string{"chore_id"},
	}
}

func (t *CompleteChoreTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	choreID, _ := args["chore_id"].(string)

	if choreID == "" {
		return ErrorResult("chore_id is required")
	}

	err := t.choreManager.CompleteChore(choreID, t.workspaceName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to complete chore: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Chore %s marked as completed", choreID))
}

type VerifyChoreTool struct {
	choreManager    *family.ChoreManager
	workspaceName   string
	canManageFamily bool
}

func NewVerifyChoreTool(familyPath, workspaceName string, canManageFamily bool) *VerifyChoreTool {
	return &VerifyChoreTool{
		choreManager:    family.NewChoreManager(familyPath),
		workspaceName:   workspaceName,
		canManageFamily: canManageFamily,
	}
}

func (t *VerifyChoreTool) Name() string {
	return "verify_chore"
}

func (t *VerifyChoreTool) Description() string {
	return "Verify a completed chore (parent only)"
}

func (t *VerifyChoreTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"chore_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the chore to verify",
			},
		},
		"required": []string{"chore_id"},
	}
}

func (t *VerifyChoreTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if !t.canManageFamily {
		return ErrorResult("only parents can verify chores")
	}

	choreID, _ := args["chore_id"].(string)

	if choreID == "" {
		return ErrorResult("chore_id is required")
	}

	err := t.choreManager.VerifyChore(choreID, t.workspaceName, t.canManageFamily)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to verify chore: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Chore %s verified", choreID))
}

type ListChoresTool struct {
	choreManager *family.ChoreManager
}

func NewListChoresTool(familyPath string) *ListChoresTool {
	return &ListChoresTool{
		choreManager: family.NewChoreManager(familyPath),
	}
}

func (t *ListChoresTool) Name() string {
	return "list_chores"
}

func (t *ListChoresTool) Description() string {
	return "List chores with optional filtering"
}

func (t *ListChoresTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"assigned_to": map[string]interface{}{
				"type":        "string",
				"description": "Filter by assignee (optional)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Filter by status: pending, completed, verified, all (default all)",
			},
		},
	}
}

func (t *ListChoresTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	assignedTo, _ := args["assigned_to"].(string)
	status, _ := args["status"].(string)

	if status == "" {
		status = "all"
	}

	chores, err := t.choreManager.ListChores(assignedTo, status)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list chores: %v", err))
	}

	if len(chores) == 0 {
		return NewToolResult("No chores found")
	}

	output := "Chores:\n"
	for _, chore := range chores {
		statusStr := "pending"
		if chore.Verified {
			statusStr = "verified"
		} else if chore.Completed {
			statusStr = "completed"
		}
		output += fmt.Sprintf("- [%s] %s (assigned to: %s, points: %d", statusStr, chore.Title, chore.AssignedTo, chore.Points)
		if chore.DueDate != "" {
			output += fmt.Sprintf(", due: %s", chore.DueDate)
		}
		output += ")\n"
	}

	return NewToolResult(output)
}

type DeleteChoreTool struct {
	choreManager    *family.ChoreManager
	canManageFamily bool
}

func NewDeleteChoreTool(familyPath string, canManageFamily bool) *DeleteChoreTool {
	return &DeleteChoreTool{
		choreManager:    family.NewChoreManager(familyPath),
		canManageFamily: canManageFamily,
	}
}

func (t *DeleteChoreTool) Name() string {
	return "delete_chore"
}

func (t *DeleteChoreTool) Description() string {
	return "Delete a chore"
}

func (t *DeleteChoreTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"chore_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the chore to delete",
			},
		},
		"required": []string{"chore_id"},
	}
}

func (t *DeleteChoreTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if !t.canManageFamily {
		return ErrorResult("only parents can delete chores")
	}

	choreID, _ := args["chore_id"].(string)

	if choreID == "" {
		return ErrorResult("chore_id is required")
	}

	err := t.choreManager.DeleteChore(choreID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to delete chore: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Chore %s deleted", choreID))
}
