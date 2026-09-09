package nodes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type FavoritesManager struct {
	mu       sync.RWMutex
	filePath string
	set      map[string]bool
}

func NewFavoritesManager(dataDir string) *FavoritesManager {
	fm := &FavoritesManager{
		filePath: filepath.Join(dataDir, "favorites.json"),
		set:      make(map[string]bool),
	}
	fm.load()
	return fm
}

func (fm *FavoritesManager) load() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	data, err := os.ReadFile(fm.filePath)
	if err != nil {
		return
	}

	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		for _, id := range list {
			fm.set[id] = true
		}
	}
}

func (fm *FavoritesManager) saveLocked() {
	var list []string
	for id, ok := range fm.set {
		if ok {
			list = append(list, id)
		}
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}

	tmp := fm.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err == nil {
		_ = os.Rename(tmp, fm.filePath)
	}
}

func (fm *FavoritesManager) IsFavorite(nodeID string) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.set[nodeID]
}

func (fm *FavoritesManager) Toggle(nodeID string) bool {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	curr := fm.set[nodeID]
	newVal := !curr
	if newVal {
		fm.set[nodeID] = true
	} else {
		delete(fm.set, nodeID)
	}
	fm.saveLocked()
	return newVal
}

func (fm *FavoritesManager) List() []string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	var list []string
	for id, ok := range fm.set {
		if ok {
			list = append(list, id)
		}
	}
	return list
}
