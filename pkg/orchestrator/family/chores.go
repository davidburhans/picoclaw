package family

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ChoreStatus string

const (
	StatusPending   ChoreStatus = "pending"
	StatusCompleted ChoreStatus = "completed"
	StatusVerified  ChoreStatus = "verified"
)

type Chore struct {
	ID          string      `json:"id"`
	Assigner    string      `json:"assigner"`
	Assignee    string      `json:"assignee"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      ChoreStatus `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	VerifiedAt  *time.Time  `json:"verified_at,omitempty"`
}

type FamilyStore struct {
	mu     sync.RWMutex
	chores map[string]*Chore
	lists  map[string]*List
}

func NewFamilyStore() *FamilyStore {
	return &FamilyStore{
		chores: make(map[string]*Chore),
		lists:  make(map[string]*List),
	}
}

func (s *FamilyStore) AssignChore(ctx context.Context, assigner, assignee, title, description string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	c := &Chore{
		ID:          id,
		Assigner:    assigner,
		Assignee:    assignee,
		Title:       title,
		Description: description,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}
	s.chores[id] = c
	return id, nil
}

func (s *FamilyStore) ListChores(ctx context.Context, user string) ([]Chore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Chore
	for _, c := range s.chores {
		// Can see chores assigned to them or assigned by them
		if c.Assignee == user || c.Assigner == user {
			result = append(result, *c)
		}
	}
	return result, nil
}

func (s *FamilyStore) CompleteChore(ctx context.Context, user, choreID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chore, ok := s.chores[choreID]
	if !ok {
		return fmt.Errorf("chore not found")
	}

	if chore.Assignee != user {
		return fmt.Errorf("unauthorized to complete this chore")
	}

	if chore.Status != StatusPending {
		return fmt.Errorf("chore is not pending")
	}

	chore.Status = StatusCompleted
	now := time.Now()
	chore.CompletedAt = &now

	return nil
}

func (s *FamilyStore) VerifyChore(ctx context.Context, user, choreID string, approved bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chore, ok := s.chores[choreID]
	if !ok {
		return fmt.Errorf("chore not found")
	}

	if chore.Assigner != user {
		return fmt.Errorf("unauthorized to verify this chore")
	}

	if chore.Status != StatusCompleted {
		return fmt.Errorf("chore is not completed yet")
	}

	if approved {
		chore.Status = StatusVerified
		now := time.Now()
		chore.VerifiedAt = &now
	} else {
		chore.Status = StatusPending
		chore.CompletedAt = nil
	}

	return nil
}
