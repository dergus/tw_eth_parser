package storage

import (
	"strings"
	"sync"
)

type RAMTransactionStorage struct {
	mu           *sync.RWMutex
	transactions []Transaction
}

func NewRAMTransactionStorage() *RAMTransactionStorage {
	return &RAMTransactionStorage{
		mu:           &sync.RWMutex{},
		transactions: make([]Transaction, 0),
	}
}

func (rts *RAMTransactionStorage) Put(t Transaction) error {
	rts.mu.Lock()
	defer rts.mu.Unlock()
	rts.transactions = append(rts.transactions, t)

	return nil
}

func (rts *RAMTransactionStorage) GetTransactionsByAddress(address string) ([]Transaction, error) {
	rts.mu.RLock()
	defer rts.mu.RUnlock()
	var ts []Transaction
	for _, t := range rts.transactions {
		if strings.EqualFold(t.From, address) || strings.EqualFold(t.To, address) {
			ts = append(ts, t)
		}
	}

	return ts, nil
}
