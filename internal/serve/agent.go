package serve

import (
	"context"
)

// MarkWorking flags a note as picked up so the user's overlay badge changes.
func (h *Handlers) MarkWorking(_ context.Context, id string) error {
	return h.store.MarkWorking(id)
}
