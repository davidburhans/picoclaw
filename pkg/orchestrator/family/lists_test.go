package family

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListsStore(t *testing.T) {
	// Red phase: FamilyStore lacks shared list capabilities
	store := NewFamilyStore()
	ctx := context.Background()

	t.Run("Create and Read List", func(t *testing.T) {
		listID, err := store.CreateList(ctx, "dad", "Groceries")
		require.NoError(t, err)
		assert.NotEmpty(t, listID)

		lists, err := store.GetLists(ctx, "kid") // Anyone in the family can see lists
		require.NoError(t, err)
		require.Len(t, lists, 1)
		assert.Equal(t, "Groceries", lists[0].Name)
		assert.Equal(t, "dad", lists[0].CreatedBy)
		assert.Empty(t, lists[0].Items)
	})

	t.Run("Add and Update Items", func(t *testing.T) {
		listID, _ := store.CreateList(ctx, "mom", "Target")

		// Kid adds an item
		itemID, err := store.AddListItem(ctx, "kid", listID, "Toys")
		require.NoError(t, err)
		assert.NotEmpty(t, itemID)

		// Mom marks it completed
		err = store.UpdateListItem(ctx, "mom", listID, itemID, true)
		require.NoError(t, err)

		lists, _ := store.GetLists(ctx, "mom")
		var targetList List
		for _, l := range lists {
			if l.ID == listID {
				targetList = l
			}
		}
		
		require.Len(t, targetList.Items, 1)
		assert.Equal(t, "Toys", targetList.Items[0].Content)
		assert.Equal(t, "kid", targetList.Items[0].AddedBy)
		assert.True(t, targetList.Items[0].Completed)
		assert.WithinDuration(t, time.Now(), *targetList.Items[0].CompletedAt, 5*time.Second)
	})

	t.Run("Delete List", func(t *testing.T) {
		listID, _ := store.CreateList(ctx, "dad", "Chores To Do")
		
		err := store.DeleteList(ctx, "kid", listID)
		assert.Error(t, err) // Kid didn't create it, but wait: is it universal delete or only creator? 
		// Actually typical family lists let anyone delete. Let's say only creator can delete it.
		assert.Contains(t, err.Error(), "unauthorized")

		err = store.DeleteList(ctx, "dad", listID)
		require.NoError(t, err)

		lists, _ := store.GetLists(ctx, "dad")
		for _, l := range lists {
			assert.NotEqual(t, listID, l.ID)
		}
	})
}
