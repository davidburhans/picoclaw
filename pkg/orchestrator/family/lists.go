package family

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ListItem struct {
	ID          string     `json:"id"`
	Content     string     `json:"content"`
	AddedBy     string     `json:"added_by"`
	Completed   bool       `json:"completed"`
	CompletedBy string     `json:"completed_by,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type List struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	Items     []ListItem `json:"items"`
}

// Ensure FamilyStore is fully implemented with Lists logic
// reusing the `FamilyStore` defined in chores.go. Wait, `lists.go` and `chores.go` are in the same package.
// I need memory structures in FamilyStore. Let's add them by creating a wrapper map in `chores.go`?
// No, I can't inject fields into a struct defined in another file directly unless I modify chores.go.
// Let's modify FamilyStore in chores.go later or here.
// Actually, it's better to modify `chores.go` to hold both or move `FamilyStore` struct definition.
// Wait! Go does not let you redefine the struct `FamilyStore` here. 
// Let's modify chores.go to have the `lists` map. Or rather, let's use `multi_replace_file_content` to add lists to FamilyStore. For now, I'll write the methods here.

// I will just put the methods here. And I will add the list fields to `FamilyStore` in chores.go via `multi_replace_file_content` right after this.

func (s *FamilyStore) CreateList(ctx context.Context, user, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	l := &List{
		ID:        id,
		Name:      name,
		CreatedBy: user,
		Items:     []ListItem{},
		CreatedAt: time.Now(),
	}

	if s.lists == nil {
		s.lists = make(map[string]*List)
	}
	s.lists[id] = l
	return id, nil
}

func (s *FamilyStore) GetLists(ctx context.Context, user string) ([]List, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []List
	for _, l := range s.lists {
		result = append(result, *l) // Return copy
	}
	return result, nil
}

func (s *FamilyStore) AddListItem(ctx context.Context, user, listID, content string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.lists[listID]
	if !ok {
		return "", fmt.Errorf("list not found")
	}

	itemID := uuid.New().String()
	item := ListItem{
		ID:        itemID,
		Content:   content,
		AddedBy:   user,
		Completed: false,
		CreatedAt: time.Now(),
	}

	l.Items = append(l.Items, item)
	return itemID, nil
}

func (s *FamilyStore) UpdateListItem(ctx context.Context, user, listID, itemID string, completed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.lists[listID]
	if !ok {
		return fmt.Errorf("list not found")
	}

	for i, item := range l.Items {
		if item.ID == itemID {
			l.Items[i].Completed = completed
			if completed {
				now := time.Now()
				l.Items[i].CompletedAt = &now
				l.Items[i].CompletedBy = user
			} else {
				l.Items[i].CompletedAt = nil
				l.Items[i].CompletedBy = ""
			}
			return nil
		}
	}
	return fmt.Errorf("item not found")
}

func (s *FamilyStore) DeleteList(ctx context.Context, user, listID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.lists[listID]
	if !ok {
		// idc, it's gone
		return nil
	}

	if l.CreatedBy != user {
		return fmt.Errorf("unauthorized to delete this list")
	}

	delete(s.lists, listID)
	return nil
}
