package tools

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/family"
)

type FamilyListsTool struct {
	listManager   *family.ListManager
	workspaceName string
}

func NewFamilyListsTool(workspacePath, familyPath, workspaceName string) *FamilyListsTool {
	return &FamilyListsTool{
		listManager:   family.NewListManager(workspacePath, familyPath),
		workspaceName: workspaceName,
	}
}

func (t *FamilyListsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list",
			},
			"item": map[string]interface{}{
				"type":        "string",
				"description": "Item to add or remove",
			},
			"quantity": map[string]interface{}{
				"type":        "integer",
				"description": "Quantity for add_to_list",
			},
			"notes": map[string]interface{}{
				"type":        "string",
				"description": "Notes for add_to_list",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name"},
	}
}

func getScope(args map[string]interface{}) string {
	scope, _ := args["scope"].(string)
	if scope != "family" {
		return "personal"
	}
	return "family"
}

type AddToListTool struct {
	listManager   *family.ListManager
	workspaceName string
}

func NewAddToListTool(workspacePath, familyPath, workspaceName string) *AddToListTool {
	return &AddToListTool{
		listManager:   family.NewListManager(workspacePath, familyPath),
		workspaceName: workspaceName,
	}
}

func (t *AddToListTool) Name() string {
	return "add_to_list"
}

func (t *AddToListTool) Description() string {
	return "Add an item to a list. Use list_name to specify which list, and item to specify what to add."
}

func (t *AddToListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list to add to",
			},
			"item": map[string]interface{}{
				"type":        "string",
				"description": "Item to add",
			},
			"quantity": map[string]interface{}{
				"type":        "integer",
				"description": "Quantity to add (optional, defaults to 1)",
			},
			"notes": map[string]interface{}{
				"type":        "string",
				"description": "Notes about the item (optional)",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name", "item"},
	}
}

func (t *AddToListTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	listName, _ := args["list_name"].(string)
	item, _ := args["item"].(string)
	notes, _ := args["notes"].(string)
	scope := getScope(args)

	quantity := 1
	if q, ok := args["quantity"].(float64); ok {
		quantity = int(q)
	}

	if listName == "" {
		return ErrorResult("list_name is required")
	}
	if item == "" {
		return ErrorResult("item is required")
	}

	err := t.listManager.AddItem(listName, item, notes, t.workspaceName, quantity, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to add item: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Added %d of %q to list %q (%s)", quantity, item, listName, scope))
}

type GetListTool struct {
	listManager *family.ListManager
}

func NewGetListTool(workspacePath, familyPath string) *GetListTool {
	return &GetListTool{
		listManager: family.NewListManager(workspacePath, familyPath),
	}
}

func (t *GetListTool) Name() string {
	return "get_list"
}

func (t *GetListTool) Description() string {
	return "Get all items from a list"
}

func (t *GetListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name"},
	}
}

func (t *GetListTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	listName, _ := args["list_name"].(string)
	scope := getScope(args)

	if listName == "" {
		return ErrorResult("list_name is required")
	}

	list, err := t.listManager.GetList(listName, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to get list: %v", err))
	}

	if len(list.Items) == 0 {
		return NewToolResult(fmt.Sprintf("List %q (%s) is empty", listName, scope))
	}

	output := fmt.Sprintf("List: %s (%s)\n", listName, scope)
	for _, item := range list.Items {
		output += fmt.Sprintf("- %s", item.Name)
		if item.Quantity > 0 {
			output += fmt.Sprintf(" (qty: %d)", item.Quantity)
		}
		if item.Notes != "" {
			output += fmt.Sprintf(" - %s", item.Notes)
		}
		if item.AddedBy != "" {
			output += fmt.Sprintf(" [added by: %s]", item.AddedBy)
		}
		output += "\n"
	}

	return NewToolResult(output)
}

type RemoveFromListTool struct {
	listManager *family.ListManager
}

func NewRemoveFromListTool(workspacePath, familyPath string) *RemoveFromListTool {
	return &RemoveFromListTool{
		listManager: family.NewListManager(workspacePath, familyPath),
	}
}

func (t *RemoveFromListTool) Name() string {
	return "remove_from_list"
}

func (t *RemoveFromListTool) Description() string {
	return "Remove an item from a list"
}

func (t *RemoveFromListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list",
			},
			"item": map[string]interface{}{
				"type":        "string",
				"description": "Item to remove",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name", "item"},
	}
}

func (t *RemoveFromListTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	listName, _ := args["list_name"].(string)
	item, _ := args["item"].(string)
	scope := getScope(args)

	if listName == "" {
		return ErrorResult("list_name is required")
	}
	if item == "" {
		return ErrorResult("item is required")
	}

	err := t.listManager.RemoveItem(listName, item, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to remove item: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Removed %q from list %q (%s)", item, listName, scope))
}

type CreateListTool struct {
	listManager     *family.ListManager
	workspaceName   string
	canManageFamily bool
}

func NewCreateListTool(workspacePath, familyPath, workspaceName string, canManageFamily bool) *CreateListTool {
	return &CreateListTool{
		listManager:     family.NewListManager(workspacePath, familyPath),
		workspaceName:   workspaceName,
		canManageFamily: canManageFamily,
	}
}

func (t *CreateListTool) Name() string {
	return "create_list"
}

func (t *CreateListTool) Description() string {
	return "Create a new list"
}

func (t *CreateListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list to create",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name"},
	}
}

func (t *CreateListTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	listName, _ := args["list_name"].(string)
	scope := getScope(args)

	if listName == "" {
		return ErrorResult("list_name is required")
	}

	if scope == "family" && !t.canManageFamily {
		return ErrorResult("only parents can create family lists")
	}

	err := t.listManager.CreateList(listName, t.workspaceName, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create list: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Created list %q (%s)", listName, scope))
}

type DeleteListTool struct {
	listManager     *family.ListManager
	canManageFamily bool
}

func NewDeleteListTool(workspacePath, familyPath string, canManageFamily bool) *DeleteListTool {
	return &DeleteListTool{
		listManager:     family.NewListManager(workspacePath, familyPath),
		canManageFamily: canManageFamily,
	}
}

func (t *DeleteListTool) Name() string {
	return "delete_list"
}

func (t *DeleteListTool) Description() string {
	return "Delete a list"
}

func (t *DeleteListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"list_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the list to delete",
			},
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
		"required": []string{"list_name"},
	}
}

func (t *DeleteListTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	listName, _ := args["list_name"].(string)
	scope := getScope(args)

	if listName == "" {
		return ErrorResult("list_name is required")
	}

	if scope == "family" && !t.canManageFamily {
		return ErrorResult("only parents can delete family lists")
	}

	err := t.listManager.DeleteList(listName, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to delete list: %v", err))
	}

	return NewToolResult(fmt.Sprintf("Deleted list %q (%s)", listName, scope))
}

type ListListsTool struct {
	listManager *family.ListManager
}

func NewListListsTool(workspacePath, familyPath string) *ListListsTool {
	return &ListListsTool{
		listManager: family.NewListManager(workspacePath, familyPath),
	}
}

func (t *ListListsTool) Name() string {
	return "list_lists"
}

func (t *ListListsTool) Description() string {
	return "List all available lists"
}

func (t *ListListsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"scope": map[string]interface{}{
				"type":        "string",
				"description": "Scope: personal or family (default personal)",
			},
		},
	}
}

func (t *ListListsTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	scope := getScope(args)

	lists, err := t.listManager.ListLists(scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list lists: %v", err))
	}

	if len(lists) == 0 {
		return NewToolResult(fmt.Sprintf("No %s lists found", scope))
	}

	output := fmt.Sprintf("%s lists:\n", scope)
	for _, list := range lists {
		output += fmt.Sprintf("- %s\n", list)
	}

	return NewToolResult(output)
}
