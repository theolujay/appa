package pipeline

import (
	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/store"
)

type Pipeline struct {
	store *store.Store
	hub   *hub.Hub
}

func New(s *store.Store, h *hub.Hub) *Pipeline {
	return &Pipeline{
		store: s,
		hub:   h,
	}
}
