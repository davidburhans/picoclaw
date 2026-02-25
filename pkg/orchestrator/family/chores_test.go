package family

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChoresStore(t *testing.T) {
	// Red phase: FamilyStore does not exist yet
	store := NewFamilyStore()
	ctx := context.Background()

	t.Run("Assign and List Chores", func(t *testing.T) {
		choreID, err := store.AssignChore(ctx, "dad", "kid", "Take out trash", "Take the recycling and trash bins out")
		require.NoError(t, err)
		assert.NotEmpty(t, choreID)

		chores, err := store.ListChores(ctx, "kid")
		require.NoError(t, err)
		require.Len(t, chores, 1)

		assert.Equal(t, "Take out trash", chores[0].Title)
		assert.Equal(t, "dad", chores[0].Assigner)
		assert.Equal(t, "kid", chores[0].Assignee)
		assert.Equal(t, StatusPending, chores[0].Status)
	})

	t.Run("Complete and Verify Chore", func(t *testing.T) {
		choreID, _ := store.AssignChore(ctx, "dad", "kid", "Clean room", "")

		// Kid completes the chore
		err := store.CompleteChore(ctx, "kid", choreID)
		require.NoError(t, err)

		chores, _ := store.ListChores(ctx, "kid")
		require.Len(t, chores, 2)
		var completedChore Chore
		for _, c := range chores {
			if c.ID == choreID {
				completedChore = c
			}
		}
		assert.Equal(t, StatusCompleted, completedChore.Status)

		// Dad verifies it
		err = store.VerifyChore(ctx, "dad", choreID, true)
		require.NoError(t, err)

		chores, _ = store.ListChores(ctx, "kid")
		var verifiedChore Chore
		for _, c := range chores {
			if c.ID == choreID {
				verifiedChore = c
			}
		}
		assert.Equal(t, StatusVerified, verifiedChore.Status)
		assert.WithinDuration(t, time.Now(), *verifiedChore.VerifiedAt, 5*time.Second)
	})

	t.Run("Unauthorized Completion", func(t *testing.T) {
		choreID, _ := store.AssignChore(ctx, "dad", "kid", "Do homework", "")

		// Sibling tries to complete it
		err := store.CompleteChore(ctx, "sibling", choreID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})
}
