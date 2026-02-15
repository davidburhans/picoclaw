package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/session"
)

type mockSessionController struct {
	rotateCalled  bool
	rotateBaseKey string
	rotateName    string
	manager       *session.SessionManager
	activeSession string
}

func (m *mockSessionController) RotateSession(ctx context.Context, baseKey, archiveName string) (string, string, error) {
	m.rotateCalled = true
	m.rotateBaseKey = baseKey
	m.rotateName = archiveName
	return "new_key", "archived_key", nil
}

func (m *mockSessionController) GetSessionManager() *session.SessionManager {
	return m.manager
}

func (m *mockSessionController) GetActiveSession(baseKey string) string {
	return m.activeSession
}

func TestSessionControlTool(t *testing.T) {
	sm := session.NewSessionManager("")
	controller := &mockSessionController{
		manager:       sm,
		activeSession: "test:123_v1",
	}
	tool := NewSessionControlTool(controller)
	tool.SetContext("test", "123")

	t.Run("start_new_session", func(t *testing.T) {
		args := map[string]interface{}{
			"action":       "start_new_session",
			"archive_name": "backup",
		}
		result := tool.Execute(context.Background(), args)

		if result.IsError {
			t.Errorf("Unexpected error: %v", result.ForLLM)
		}
		if !controller.rotateCalled {
			t.Error("RotateSession was not called")
		}
		if controller.rotateBaseKey != "test:123" {
			t.Errorf("Expected baseKey test:123, got %s", controller.rotateBaseKey)
		}
		if controller.rotateName != "backup" {
			t.Errorf("Expected archiveName backup, got %s", controller.rotateName)
		}
		if !strings.Contains(result.ForLLM, "new_key") {
			t.Errorf("Expected result to contain new_key, got %s", result.ForLLM)
		}
	})

	t.Run("get_session_info", func(t *testing.T) {
		args := map[string]interface{}{
			"action": "get_session_info",
		}
		result := tool.Execute(context.Background(), args)

		if result.IsError {
			t.Errorf("Unexpected error: %v", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "test:123_v1") {
			t.Errorf("Expected result to contain active session key, got %s", result.ForLLM)
		}
	})
}
