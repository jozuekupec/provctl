package ui

import (
	"context"
	"time"
)

const readTimeout = 15 * time.Second

// opSlot owns one replaceable read operation. A generation is retained in its
// response so a dependency that finishes after cancellation cannot update a
// newer selection.
type opSlot struct {
	cancel     context.CancelFunc
	generation uint64
}

func (slot *opSlot) start() (context.Context, uint64) {
	slot.invalidate()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	slot.cancel = cancel
	return ctx, slot.generation
}

func (slot *opSlot) invalidate() {
	if slot.cancel != nil {
		slot.cancel()
		slot.cancel = nil
	}
	slot.generation++
}

func (slot *opSlot) stale(generation uint64) bool {
	return generation != slot.generation
}

func (slot *opSlot) finish(generation uint64) {
	if !slot.stale(generation) {
		slot.cancel = nil
	}
}
