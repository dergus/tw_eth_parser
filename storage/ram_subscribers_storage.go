package storage

import (
	"strings"
	"sync"
)

type RAMSubscribersStorage struct {
	mu        *sync.RWMutex
	addresses map[string]struct{}
}

func NewRAMSubscribersStorage() *RAMSubscribersStorage {
	return &RAMSubscribersStorage{
		mu:        &sync.RWMutex{},
		addresses: map[string]struct{}{},
	}
}

func (s *RAMSubscribersStorage) Put(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.addresses[strings.ToLower(address)] = struct{}{}

	return nil
}

func (s *RAMSubscribersStorage) Exists(address string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.addresses[strings.ToLower(address)]

	return ok
}
