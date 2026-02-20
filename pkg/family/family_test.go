package family

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListManager(t *testing.T) {
	tmpDir := t.TempDir()

	lm := NewListManager(
		filepath.Join(tmpDir, "personal"),
		filepath.Join(tmpDir, "family"),
	)

	if err := lm.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	// Test creating a personal list
	if err := lm.AddItem("tasks", "Buy milk", "", "dave", 1, "personal"); err != nil {
		t.Fatalf("AddItem failed: %v", err)
	}

	// Test getting a list
	list, err := lm.GetList("tasks", "personal")
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if len(list.Items) != 1 || list.Items[0].Name != "Buy milk" {
		t.Errorf("Expected list with 1 item 'Buy milk', got %v", list.Items)
	}

	// Test removing from list
	if err := lm.RemoveItem("tasks", "Buy milk", "personal"); err != nil {
		t.Fatalf("RemoveItem failed: %v", err)
	}

	list, _ = lm.GetList("tasks", "personal")
	if len(list.Items) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list.Items))
	}

	// Test family list
	if err := lm.AddItem("groceries", "Apples", "", "dave", 3, "family"); err != nil {
		t.Fatalf("AddItem to family failed: %v", err)
	}

	list, err = lm.GetList("groceries", "family")
	if err != nil {
		t.Fatalf("GetList family failed: %v", err)
	}

	if len(list.Items) != 1 || list.Items[0].Name != "Apples" {
		t.Errorf("Expected family list with 1 item, got %v", list.Items)
	}

	// Test list lists
	personalLists, _ := lm.ListLists("personal")
	if len(personalLists) != 1 || personalLists[0] != "tasks" {
		t.Errorf("Expected personal lists [tasks], got %v", personalLists)
	}

	familyLists, _ := lm.ListLists("family")
	if len(familyLists) != 1 || familyLists[0] != "groceries" {
		t.Errorf("Expected family lists [groceries], got %v", familyLists)
	}

	// Cleanup
	os.RemoveAll(tmpDir)
}

func TestChoreManager(t *testing.T) {
	tmpDir := t.TempDir()

	cm := NewChoreManager(tmpDir)

	if err := cm.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile failed: %v", err)
	}

	// Test assigning a chore
	chore, err := cm.AssignChore("timmy", "Clean room", "Make your bed too", "2026-02-21", 10, "dave")
	if err != nil {
		t.Fatalf("AssignChore failed: %v", err)
	}

	if chore.Title != "Clean room" || chore.Points != 10 {
		t.Errorf("Expected chore with title 'Clean room' and 10 points, got %+v", chore)
	}

	// Test listing chores
	chores, err := cm.ListChores("", "all")
	if err != nil {
		t.Fatalf("ListChores failed: %v", err)
	}

	if len(chores) != 1 {
		t.Errorf("Expected 1 chore, got %d", len(chores))
	}

	// Test completing a chore
	err = cm.CompleteChore(chore.ID, "timmy")
	if err != nil {
		t.Fatalf("CompleteChore failed: %v", err)
	}

	// Test verifying a chore
	err = cm.VerifyChore(chore.ID, "dave", true)
	if err != nil {
		t.Fatalf("VerifyChore failed: %v", err)
	}

	verifiedChores, _ := cm.ListChores("", "verified")
	if len(verifiedChores) != 1 {
		t.Errorf("Expected 1 verified chore, got %d", len(verifiedChores))
	}

	// Cleanup
	os.RemoveAll(tmpDir)
}
