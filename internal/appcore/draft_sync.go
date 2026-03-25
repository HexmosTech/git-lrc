package appcore

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrDraftFrozen       = errors.New("draft is frozen")
	ErrDraftStaleVersion = errors.New("stale draft version")
)

type draftSource string

const (
	draftSourceSystem   draftSource = "system"
	draftSourceWeb      draftSource = "web"
	draftSourceTerminal draftSource = "terminal"
	draftSourceEditor   draftSource = "editor"
)

type draftSnapshot struct {
	Text      string `json:"text"`
	Version   int64  `json:"version"`
	Source    string `json:"source"`
	Frozen    bool   `json:"frozen"`
	UpdatedAt string `json:"updatedAt"`
}

type draftHub struct {
	mu      sync.RWMutex
	text    string
	version int64
	source  draftSource
	frozen  bool
	watch   map[chan draftSnapshot]struct{}
}

func newDraftHub(initial string) *draftHub {
	return &draftHub{
		text:    initial,
		version: 1,
		source:  draftSourceSystem,
		watch:   make(map[chan draftSnapshot]struct{}),
	}
}

func (h *draftHub) Snapshot() draftSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return draftSnapshot{
		Text:      h.text,
		Version:   h.version,
		Source:    string(h.source),
		Frozen:    h.frozen,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (h *draftHub) Update(text string, source draftSource, expectedVersion int64) (draftSnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.frozen {
		return h.snapshotLocked(), ErrDraftFrozen
	}
	if expectedVersion > 0 && expectedVersion < h.version {
		return h.snapshotLocked(), fmt.Errorf("%w: got %d current %d", ErrDraftStaleVersion, expectedVersion, h.version)
	}

	h.version++
	h.text = text
	h.source = source
	snap := h.snapshotLocked()
	h.broadcastLocked(snap)
	return snap, nil
}

func (h *draftHub) Freeze() draftSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frozen = true
	snap := h.snapshotLocked()
	h.broadcastLocked(snap)
	return snap
}

func (h *draftHub) Subscribe() (<-chan draftSnapshot, func()) {
	ch := make(chan draftSnapshot, 16)

	h.mu.Lock()
	h.watch[ch] = struct{}{}
	snap := h.snapshotLocked()
	h.mu.Unlock()

	ch <- snap

	unsubscribe := func() {
		h.mu.Lock()
		if _, ok := h.watch[ch]; ok {
			delete(h.watch, ch)
			close(ch)
		}
		h.mu.Unlock()
	}

	return ch, unsubscribe
}

func (h *draftHub) snapshotLocked() draftSnapshot {
	return draftSnapshot{
		Text:      h.text,
		Version:   h.version,
		Source:    string(h.source),
		Frozen:    h.frozen,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (h *draftHub) broadcastLocked(snap draftSnapshot) {
	for ch := range h.watch {
		select {
		case ch <- snap:
		default:
		}
	}
}
