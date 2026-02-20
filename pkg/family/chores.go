package family

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Chore struct {
	ID          string `json:"id"`
	AssignedTo  string `json:"assigned_to"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Points      int    `json:"points"`
	DueDate     string `json:"due_date,omitempty"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at,omitempty"`
	Verified    bool   `json:"verified"`
	VerifiedBy  string `json:"verified_by,omitempty"`
	VerifiedAt  string `json:"verified_at,omitempty"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
}

type ChoreManager struct {
	choresPath string
	mu         sync.RWMutex
}

func NewChoreManager(familyPath string) *ChoreManager {
	return &ChoreManager{
		choresPath: filepath.Join(familyPath, "chores.json"),
	}
}

func (m *ChoreManager) EnsureFile() error {
	dir := filepath.Dir(m.choresPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating family dir: %w", err)
	}
	if _, err := os.Stat(m.choresPath); os.IsNotExist(err) {
		return os.WriteFile(m.choresPath, []byte("[]"), 0644)
	}
	return nil
}

func (m *ChoreManager) AssignChore(assignedTo, title, description, dueDate string, points int, createdBy string) (*Chore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnsureFile()

	chores, err := m.loadChores()
	if err != nil {
		return nil, err
	}

	chore := &Chore{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		AssignedTo:  assignedTo,
		Title:       title,
		Description: description,
		Points:      points,
		DueDate:     dueDate,
		Completed:   false,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	chores = append(chores, *chore)
	if err := m.saveChores(chores); err != nil {
		return nil, err
	}

	return chore, nil
}

func (m *ChoreManager) CompleteChore(choreID, completedBy string) error {
	if completedBy == "" {
		return fmt.Errorf("completedBy is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnsureFile()

	chores, err := m.loadChores()
	if err != nil {
		return err
	}

	for i := range chores {
		if chores[i].ID == choreID {
			if chores[i].AssignedTo != completedBy {
				return fmt.Errorf("not authorized to complete this chore")
			}
			chores[i].Completed = true
			chores[i].CompletedAt = time.Now().Format(time.RFC3339)
			return m.saveChores(chores)
		}
	}

	return fmt.Errorf("chore not found: %s", choreID)
}

func (m *ChoreManager) VerifyChore(choreID, verifiedBy string, canManageFamily bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnsureFile()

	chores, err := m.loadChores()
	if err != nil {
		return err
	}

	for i := range chores {
		if chores[i].ID == choreID {
			if !canManageFamily {
				return fmt.Errorf("only parents can verify chores")
			}
			if !chores[i].Completed {
				return fmt.Errorf("chore must be completed before verification")
			}
			chores[i].Verified = true
			chores[i].VerifiedBy = verifiedBy
			chores[i].VerifiedAt = time.Now().Format(time.RFC3339)
			return m.saveChores(chores)
		}
	}

	return fmt.Errorf("chore not found: %s", choreID)
}

func (m *ChoreManager) ListChores(assignedTo, status string) ([]Chore, error) {
	m.EnsureFile()

	chores, err := m.loadChores()
	if err != nil {
		return nil, err
	}

	var result []Chore
	for _, chore := range chores {
		if assignedTo != "" && chore.AssignedTo != assignedTo {
			continue
		}

		switch status {
		case "pending":
			if !chore.Completed {
				result = append(result, chore)
			}
		case "completed":
			if chore.Completed && !chore.Verified {
				result = append(result, chore)
			}
		case "verified":
			if chore.Verified {
				result = append(result, chore)
			}
		default:
			result = append(result, chore)
		}
	}

	return result, nil
}

func (m *ChoreManager) DeleteChore(choreID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	chores, err := m.loadChores()
	if err != nil {
		return err
	}

	found := false
	newChores := []Chore{}
	for _, chore := range chores {
		if chore.ID != choreID {
			newChores = append(newChores, chore)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("chore not found: %s", choreID)
	}

	return m.saveChores(newChores)
}

func (m *ChoreManager) loadChores() ([]Chore, error) {
	data, err := os.ReadFile(m.choresPath)
	if err != nil {
		return nil, err
	}
	var chores []Chore
	if err := json.Unmarshal(data, &chores); err != nil {
		return nil, err
	}
	return chores, nil
}

func (m *ChoreManager) saveChores(chores []Chore) error {
	data, err := json.MarshalIndent(chores, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.choresPath, data, 0644)
}
