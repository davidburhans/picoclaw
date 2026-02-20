package family

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ListItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity,omitempty"`
	Notes    string `json:"notes,omitempty"`
	AddedBy  string `json:"added_by,omitempty"`
}

type List struct {
	Name      string     `json:"name"`
	Items     []ListItem `json:"items"`
	CreatedBy string     `json:"created_by,omitempty"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
}

type ListManager struct {
	personalDir string
	familyDir   string
	mu          sync.RWMutex
}

func NewListManager(workspacePath string, familyPath string) *ListManager {
	return &ListManager{
		personalDir: filepath.Join(workspacePath, "lists"),
		familyDir:   filepath.Join(familyPath, "lists"),
	}
}

func (m *ListManager) EnsureDirectories() error {
	if err := os.MkdirAll(m.personalDir, 0755); err != nil {
		return fmt.Errorf("creating personal lists dir: %w", err)
	}
	if err := os.MkdirAll(m.familyDir, 0755); err != nil {
		return fmt.Errorf("creating family lists dir: %w", err)
	}
	return nil
}

func (m *ListManager) listPath(listName string, scope string) string {
	dir := m.familyDir
	if scope == "personal" {
		dir = m.personalDir
	}
	return filepath.Join(dir, sanitizeFileName(listName)+".json")
}

func sanitizeFileName(name string) string {
	result := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == ' ' {
			result += string(c)
		}
	}
	return result
}

func (m *ListManager) AddItem(listName, itemName, notes, addedBy string, quantity int, scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnsureDirectories()

	path := m.listPath(listName, scope)
	list, err := m.loadList(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// If list doesn't exist or is nil, create a new one
	if list == nil {
		list = &List{
			Name: listName,
		}
	}
	if list.Items == nil {
		list.Items = []ListItem{}
	}

	list.Items = append(list.Items, ListItem{
		Name:     itemName,
		Quantity: quantity,
		Notes:    notes,
		AddedBy:  addedBy,
	})

	return m.saveList(path, list)
}

func (m *ListManager) RemoveItem(listName, itemName string, scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := m.listPath(listName, scope)
	list, err := m.loadList(path)
	if err != nil {
		return err
	}

	found := false
	newItems := []ListItem{}
	for _, item := range list.Items {
		if item.Name != itemName {
			newItems = append(newItems, item)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("item not found: %s", itemName)
	}

	list.Items = newItems
	return m.saveList(path, list)
}

func (m *ListManager) GetList(listName string, scope string) (*List, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	path := m.listPath(listName, scope)
	return m.loadList(path)
}

func (m *ListManager) ListLists(scope string) ([]string, error) {
	dir := m.familyDir
	if scope == "personal" {
		dir = m.personalDir
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var lists []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			lists = append(lists, entry.Name()[:len(entry.Name())-5])
		}
	}
	return lists, nil
}

func (m *ListManager) CreateList(listName, createdBy, scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnsureDirectories()

	path := m.listPath(listName, scope)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("list already exists: %s", listName)
	}

	list := &List{
		Name:      listName,
		Items:     []ListItem{},
		CreatedBy: createdBy,
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	return m.saveList(path, list)
}

func (m *ListManager) DeleteList(listName, scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := m.listPath(listName, scope)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("deleting list: %w", err)
	}
	return nil
}

func (m *ListManager) loadList(path string) (*List, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list List
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (m *ListManager) saveList(path string, list *List) error {
	list.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *ListManager) FamilyPath() string {
	return m.familyDir
}
