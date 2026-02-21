package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/utils"
)

// validatePath ensures the given path is within the workspace or allowed external paths if restrict is true.
func validatePath(path, workspace string, allowedExternalPaths []string, restrict bool) (string, error) {
	if workspace == "" {
		return path, nil
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath, err = filepath.Abs(filepath.Join(absWorkspace, path))
		if err != nil {
			return "", fmt.Errorf("failed to resolve file path: %w", err)
		}
	}

	if restrict {
		// Check if it's within workspace or any allowed external path
		isAllowed := isWithinWorkspace(absPath, absWorkspace)
		if !isAllowed {
			for _, p := range allowedExternalPaths {
				if absP, err := filepath.Abs(utils.ExpandHome(p)); err == nil {
					if isWithinWorkspace(absPath, absP) {
						isAllowed = true
						break
					}
				}
			}
		}

		if !isAllowed {
			return "", fmt.Errorf("access denied: path is outside the workspace and allowed external paths")
		}

		var resolved string
		workspaceReal := absWorkspace
		if resolved, err = filepath.EvalSymlinks(absWorkspace); err == nil {
			workspaceReal = resolved
		}

		allowedReals := make([]string, 0, len(allowedExternalPaths))
		for _, p := range allowedExternalPaths {
			if absP, err := filepath.Abs(utils.ExpandHome(p)); err == nil {
				if resolved, err := filepath.EvalSymlinks(absP); err == nil {
					allowedReals = append(allowedReals, resolved)
				} else {
					allowedReals = append(allowedReals, absP)
				}
			}
		}

		if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
			isAllowed = isWithinWorkspace(resolved, workspaceReal)
			if !isAllowed {
				for _, r := range allowedReals {
					if isWithinWorkspace(resolved, r) {
						isAllowed = true
						break
					}
				}
			}
			if !isAllowed {
				return "", fmt.Errorf("access denied: symlink resolves outside workspace and allowed external paths")
			}
		} else if os.IsNotExist(err) {
			if parentResolved, err := resolveExistingAncestor(filepath.Dir(absPath)); err == nil {
				isAllowed = isWithinWorkspace(parentResolved, workspaceReal)
				if !isAllowed {
					for _, r := range allowedReals {
						if isWithinWorkspace(parentResolved, r) {
							isAllowed = true
							break
						}
					}
				}
				if !isAllowed {
					return "", fmt.Errorf("access denied: symlink resolves outside workspace and allowed external paths")

				}
			} else if !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to resolve path: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to resolve path: %w", err)
		}
	}

	return absPath, nil
}

func resolveExistingAncestor(path string) (string, error) {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return resolved, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(current) == current {
			return "", os.ErrNotExist
		}
	}
}

func isWithinWorkspace(candidate, workspace string) bool {
	rel, err := filepath.Rel(filepath.Clean(workspace), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

type ReadFileTool struct {
	workspace    string
	allowedPaths []string
	restrict     bool
}

func NewReadFileTool(workspace string, allowedPaths []string, restrict bool) *ReadFileTool {
	return &ReadFileTool{workspace: workspace, allowedPaths: allowedPaths, restrict: restrict}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file with optional paging and truncation"
}

func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read",
			},
			"max_bytes": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of bytes to read (default: 5000). Use to avoid consuming too much context.",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "The byte offset to start reading from (default: 0). Use for paging through large files.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	maxBytes := int64(5000)
	if mb, ok := args["max_bytes"]; ok {
		switch v := mb.(type) {
		case int:
			maxBytes = int64(v)
		case int64:
			maxBytes = v
		case float64:
			maxBytes = int64(v)
		}
	}

	offset := int64(0)
	if off, ok := args["offset"]; ok {
		switch v := off.(type) {
		case int:
			offset = int64(v)
		case int64:
			offset = v
		case float64:
			offset = int64(v)
		}
	}

	resolvedPath, err := validatePath(path, t.workspace, t.allowedPaths, t.restrict)
	if err != nil {
		return ErrorResult(err.Error())
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to stat file: %v", err))
	}

	totalSize := info.Size()
	if offset > totalSize {
		return ErrorResult(fmt.Sprintf("offset %d is beyond file size %d", offset, totalSize))
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ErrorResult(fmt.Sprintf("failed to seek to offset %d: %v", offset, err))
	}

	lr := io.LimitReader(f, maxBytes)
	content, err := io.ReadAll(lr)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	bytesRead := int64(len(content))
	result := string(content)

	if totalSize > offset+bytesRead {
		result += fmt.Sprintf("\n\n[TRUNCATED: showing bytes %d-%d of %d total. Use the 'offset' parameter to read more.]", offset, offset+bytesRead, totalSize)
	}

	return NewToolResult(result)
}

type WriteFileTool struct {
	workspace    string
	allowedPaths []string
	restrict     bool
}

func NewWriteFileTool(workspace string, allowedPaths []string, restrict bool) *WriteFileTool {
	return &WriteFileTool{workspace: workspace, allowedPaths: allowedPaths, restrict: restrict}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file"
}

func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return ErrorResult("content is required")
	}

	resolvedPath, err := validatePath(path, t.workspace, t.allowedPaths, t.restrict)
	if err != nil {
		return ErrorResult(err.Error())
	}

	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err))
	}

	if err := os.WriteFile(resolvedPath, []byte(content), 0o644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}

	return SilentResult(fmt.Sprintf("File written: %s", path))
}

type ListDirTool struct {
	workspace    string
	allowedPaths []string
	restrict     bool
}

func NewListDirTool(workspace string, allowedPaths []string, restrict bool) *ListDirTool {
	return &ListDirTool{workspace: workspace, allowedPaths: allowedPaths, restrict: restrict}
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List files and directories in a path"
}

func (t *ListDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		path = "."
	}

	resolvedPath, err := validatePath(path, t.workspace, t.allowedPaths, t.restrict)
	if err != nil {
		return ErrorResult(err.Error())
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read directory: %v", err))
	}

	result := ""
	for _, entry := range entries {
		if entry.IsDir() {
			result += "DIR:  " + entry.Name() + "\n"
		} else {
			result += "FILE: " + entry.Name() + "\n"
		}
	}

	return NewToolResult(result)
}
