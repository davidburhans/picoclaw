package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFilesystemTool_ReadFile_Success verifies successful file reading
func TestFilesystemTool_ReadFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0o644)

	tool := NewReadFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path": testFile,
	}

	result := tool.Execute(ctx, args)

	// Success should not be an error
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// ForLLM should contain file content
	if !strings.Contains(result.ForLLM, "test content") {
		t.Errorf("Expected ForLLM to contain 'test content', got: %s", result.ForLLM)
	}

	// ReadFile returns NewToolResult which only sets ForLLM, not ForUser
	// This is the expected behavior - file content goes to LLM, not directly to user
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for NewToolResult, got: %s", result.ForUser)
	}
}

// TestFilesystemTool_ReadFile_NotFound verifies error handling for missing file
func TestFilesystemTool_ReadFile_NotFound(t *testing.T) {
	tool := NewReadFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/nonexistent_file_12345.txt",
	}

	result := tool.Execute(ctx, args)

	// Failure should be marked as error
	if !result.IsError {
		t.Errorf("Expected error for missing file, got IsError=false")
	}

	// Should contain error message
	if !strings.Contains(result.ForLLM, "failed to read") && !strings.Contains(result.ForUser, "failed to read") {
		t.Errorf("Expected error message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestFilesystemTool_ReadFile_MissingPath verifies error handling for missing path
func TestFilesystemTool_ReadFile_MissingPath(t *testing.T) {
	tool := NewReadFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// Should return error result
	if !result.IsError {
		t.Errorf("Expected error when path is missing")
	}

	// Should mention required parameter
	if !strings.Contains(result.ForLLM, "path is required") && !strings.Contains(result.ForUser, "path is required") {
		t.Errorf("Expected 'path is required' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestFilesystemTool_WriteFile_Success verifies successful file writing
func TestFilesystemTool_WriteFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "newfile.txt")

	tool := NewWriteFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": "hello world",
	}

	result := tool.Execute(ctx, args)

	// Success should not be an error
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// WriteFile returns SilentResult
	if !result.Silent {
		t.Errorf("Expected Silent=true for WriteFile, got false")
	}

	// ForUser should be empty (silent result)
	if result.ForUser != "" {
		t.Errorf("Expected ForUser to be empty for SilentResult, got: %s", result.ForUser)
	}

	// Verify file was actually written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("Expected file content 'hello world', got: %s", string(content))
	}
}

// TestFilesystemTool_WriteFile_CreateDir verifies directory creation
func TestFilesystemTool_WriteFile_CreateDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "newfile.txt")

	tool := NewWriteFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path":    testFile,
		"content": "test",
	}

	result := tool.Execute(ctx, args)

	// Success should not be an error
	if result.IsError {
		t.Errorf("Expected success with directory creation, got IsError=true: %s", result.ForLLM)
	}

	// Verify directory was created and file written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "test" {
		t.Errorf("Expected file content 'test', got: %s", string(content))
	}
}

// TestFilesystemTool_WriteFile_MissingPath verifies error handling for missing path
func TestFilesystemTool_WriteFile_MissingPath(t *testing.T) {
	tool := NewWriteFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"content": "test",
	}

	result := tool.Execute(ctx, args)

	// Should return error result
	if !result.IsError {
		t.Errorf("Expected error when path is missing")
	}
}

// TestFilesystemTool_WriteFile_MissingContent verifies error handling for missing content
func TestFilesystemTool_WriteFile_MissingContent(t *testing.T) {
	tool := NewWriteFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/tmp/test.txt",
	}

	result := tool.Execute(ctx, args)

	// Should return error result
	if !result.IsError {
		t.Errorf("Expected error when content is missing")
	}

	// Should mention required parameter
	if !strings.Contains(result.ForLLM, "content is required") &&
		!strings.Contains(result.ForUser, "content is required") {
		t.Errorf("Expected 'content is required' message, got ForLLM: %s", result.ForLLM)
	}
}

// TestFilesystemTool_ListDir_Success verifies successful directory listing
func TestFilesystemTool_ListDir_Success(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content"), 0o644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755)

	tool := NewListDirTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path": tmpDir,
	}

	result := tool.Execute(ctx, args)

	// Success should not be an error
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// Should list files and directories
	if !strings.Contains(result.ForLLM, "file1.txt") || !strings.Contains(result.ForLLM, "file2.txt") {
		t.Errorf("Expected files in listing, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "subdir") {
		t.Errorf("Expected subdir in listing, got: %s", result.ForLLM)
	}
}

// TestFilesystemTool_ListDir_NotFound verifies error handling for non-existent directory
func TestFilesystemTool_ListDir_NotFound(t *testing.T) {
	tool := NewListDirTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{
		"path": "/nonexistent_directory_12345",
	}

	result := tool.Execute(ctx, args)

	// Failure should be marked as error
	if !result.IsError {
		t.Errorf("Expected error for non-existent directory, got IsError=false")
	}

	// Should contain error message
	if !strings.Contains(result.ForLLM, "failed to read") && !strings.Contains(result.ForUser, "failed to read") {
		t.Errorf("Expected error message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestFilesystemTool_ListDir_DefaultPath verifies default to current directory
func TestFilesystemTool_ListDir_DefaultPath(t *testing.T) {
	tool := NewListDirTool("", nil, false)
	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// Should use "." as default path
	if result.IsError {
		t.Errorf("Expected success with default path '.', got IsError=true: %s", result.ForLLM)
	}
}

// Block paths that look inside workspace but point outside via symlink.
func TestFilesystemTool_ReadFile_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	link := filepath.Join(workspace, "leak.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	tool := NewReadFileTool(workspace, nil, true)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": link,
	})

	if !result.IsError {
		t.Fatalf("expected symlink escape to be blocked")
	}
	if !strings.Contains(result.ForLLM, "symlink resolves outside workspace") {
		t.Fatalf("expected symlink escape error, got: %s", result.ForLLM)
	}
}

func TestFilesystemTool_AllowedExternalPaths(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	external := filepath.Join(root, "external")
	forbidden := filepath.Join(root, "forbidden")

	os.MkdirAll(workspace, 0755)
	os.MkdirAll(external, 0755)
	os.MkdirAll(forbidden, 0755)

	os.WriteFile(filepath.Join(workspace, "in.txt"), []byte("in"), 0644)
	os.WriteFile(filepath.Join(external, "out.txt"), []byte("out"), 0644)
	os.WriteFile(filepath.Join(forbidden, "secret.txt"), []byte("secret"), 0644)

	allowedPaths := []string{external}
	tool := NewReadFileTool(workspace, allowedPaths, true)

	ctx := context.Background()

	// Test 1: Access within workspace
	res1 := tool.Execute(ctx, map[string]interface{}{"path": "in.txt"})
	if res1.IsError {
		t.Errorf("expected success for in.txt, got error: %s", res1.ForLLM)
	}

	// Test 2: Access within allowed external path
	res2 := tool.Execute(ctx, map[string]interface{}{"path": filepath.Join(external, "out.txt")})
	if res2.IsError {
		t.Errorf("expected success for external out.txt, got error: %s", res2.ForLLM)
	}

	// Test 3: Access within forbidden external path
	res3 := tool.Execute(ctx, map[string]interface{}{"path": filepath.Join(forbidden, "secret.txt")})
	if !res3.IsError {
		t.Errorf("expected error for forbidden secret.txt, got success")
	}
}

// TestFilesystemTool_ReadFile_Truncation verifies truncation with max_bytes
func TestFilesystemTool_ReadFile_Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")
	content := strings.Repeat("a", 10000)
	os.WriteFile(testFile, []byte(content), 0644)

	tool := NewReadFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]interface{}{
		"path":      testFile,
		"max_bytes": 1000,
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success, got Error: %s", result.ForLLM)
	}

	// Should contain content and truncation message
	if !strings.Contains(result.ForLLM, "[TRUNCATED") {
		t.Errorf("Expected truncation message, got: %s", result.ForLLM)
	}

	// Should contain exactly 1000 characters plus header/footer (in this case footer)
	expectedPrefix := strings.Repeat("a", 1000)
	if !strings.HasPrefix(result.ForLLM, expectedPrefix) {
		t.Errorf("Expected content to start with 1000 'a's")
	}
}

// TestFilesystemTool_ReadFile_Offset verifies paging with offset
func TestFilesystemTool_ReadFile_Offset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "paging.txt")
	content := "0123456789"
	os.WriteFile(testFile, []byte(content), 0644)

	tool := NewReadFileTool("", nil, false)
	ctx := context.Background()
	args := map[string]interface{}{
		"path":      testFile,
		"offset":    5,
		"max_bytes": 2,
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success, got Error: %s", result.ForLLM)
	}

	// Should show bytes 5-7 (offset 5, read 2)
	expectedContent := "56"
	if !strings.HasPrefix(result.ForLLM, expectedContent) {
		t.Errorf("Expected content '56', got: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "[TRUNCATED: showing bytes 5-7 of 10 total") {
		t.Errorf("Expected correct truncation message, got: %s", result.ForLLM)
	}
}
