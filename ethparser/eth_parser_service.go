package ethparser

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"tw_eth_parser/ethclient"
	"tw_eth_parser/storage"
)

type SubscribersStorage interface {
	Put(address string) error
	Exists(address string) bool
}

type TransactionStorage interface {
	Put(t storage.Transaction) error
	GetTransactionsByAddress(address string) ([]storage.Transaction, error)
}

type Config struct {
	EthURL                  string
	BlockProcessingInterval time.Duration
}

type EthAPIClient interface {
	GetBlockTransactions(ctx context.Context, blockNum string) ([]ethclient.Transaction, error)
	GetLatestBlockNumber(ctx context.Context) (string, error)
}

type EthParserService struct {
	conf               Config
	ethClient          EthAPIClient
	currentBlockNumber uint64
	subscribers        SubscribersStorage
	transactionStorage TransactionStorage
}

func NewEthParserService(conf Config, ec EthAPIClient, ts TransactionStorage, ss SubscribersStorage) *EthParserService {
	return &EthParserService{
		conf:               conf,
		ethClient:          ec,
		subscribers:        ss,
		transactionStorage: ts,
	}
}

func (p *EthParserService) Subscribe(address string) {
	p.subscribers.Put(strings.ToLower(address))
}

func (p *EthParserService) GetCurrentBlockNumber() uint64 {
	return atomic.LoadUint64(&p.currentBlockNumber)
}

func (p *EthParserService) setCurrentBlockNumber(v uint64) {
	atomic.StoreUint64(&p.currentBlockNumber, v)
}

func (p *EthParserService) initCurrentBlockNumber(ctx context.Context) error {
	n, err := p.ethClient.GetLatestBlockNumber(ctx)
	if err != nil {
		return err
	}

	latestBlockNumInt, err := strconv.ParseUint(n, 0, 64)
	if err != nil {
		return fmt.Errorf("couldn't convert block number value '%v' %w", n, err)
	}

	p.setCurrentBlockNumber(latestBlockNumInt)

	return nil
}

func (p *EthParserService) init(ctx context.Context) error {
	if err := p.initCurrentBlockNumber(ctx); err != nil {
		return fmt.Errorf("couldn't init current block number: %w", err)
	}

	if err := p.processLatestBlock(ctx); err != nil {
		return fmt.Errorf("couldn't process latest block: %w", err)
	}

	return nil
}

func (p *EthParserService) Run(ctx context.Context) (error, context.CancelFunc) {
	if err := p.init(ctx); err != nil {
		return fmt.Errorf("couldn't init parser: %w", err), nil
	}

	ctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(p.conf.BlockProcessingInterval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if err := p.processLatestBlock(ctx); err != nil {
					log.Printf("couldn't process latest block: %v", err)
				} else {
					log.Printf("latest processed block number: %v\n", p.GetCurrentBlockNumber())
				}
			default:
			}
		}
	}()

	return nil, cancel
}

func (p *EthParserService) processLatestBlock(ctx context.Context) error {
	latestBlockNum := p.GetCurrentBlockNumber()
	latestBlockNum++
	numHex := fmt.Sprintf("0x%x", latestBlockNum)
	log.Printf("getting transactions for block %v\n", latestBlockNum)
	transactions, err := p.ethClient.GetBlockTransactions(ctx, numHex)
	if err != nil {
		return err
	}

	for _, t := range transactions {
		if p.subscribers.Exists(t.To) || p.subscribers.Exists(t.From) {
			st := storage.Transaction{
				Hash:  t.Hash,
				From:  t.From,
				To:    t.To,
				Value: t.Value,
			}
			if err := p.transactionStorage.Put(st); err != nil {
				return fmt.Errorf("couldn't store transaction: %w", err)
			}
		}
	}

	p.setCurrentBlockNumber(latestBlockNum)

	return nil
}

func (p *EthParserService) Exists(address string) bool {
	return p.subscribers.Exists(address)
}

func (p *EthParserService) GetTransactionsByAddress(address string) ([]storage.Transaction, error) {
	return p.transactionStorage.GetTransactionsByAddress(address)
}
