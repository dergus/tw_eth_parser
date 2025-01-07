package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	conf := Config{
		EthURL:                  "https://cloudflare-eth.com",
		BlockProcessingInterval: 15 * time.Second,
		EthAPITimeout:           10 * time.Second,
	}

	ctx := context.Background()
	defer ctx.Done()

	ts := &RAMTransactionStorage{
		mu: &sync.RWMutex{},
	}

	ss := &RAMSubscribersStorage{
		mu:        &sync.RWMutex{},
		addresses: map[string]struct{}{},
	}

	ethParser := NewEthParser(conf, ts, ss)

	if err := ethParser.initCurrentBlockNumber(ctx); err != nil {
		log.Fatalf("couldn't init current block number: %v\n", err)
	}

	if err := ethParser.processLatestBlock(ctx); err != nil {
		log.Fatalf("couldn't process latest block: %v\n", err)
	} else {
		log.Printf("latest processed block number: %v\n", ethParser.GetCurrentBlockNumber())
	}

	ethParser.start(ctx)
	handler := NewAPIHandler(ethParser, ts)
	// API routes
	http.HandleFunc("GET /api/block/current", handler.getCurrentBlock)
	http.HandleFunc("POST /api/subscribe", handler.subscribe)
	http.HandleFunc("GET /api/transactions", handler.getTransactions)

	// Start server
	port := ":8080"
	log.Printf("Server starting on port %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

type SubscribeRequest struct {
	Address string `json:"address"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type APIHandler struct {
	parser             *EthParser
	transactionStorage TransactionStorage
}

func NewAPIHandler(p *EthParser, ts TransactionStorage) *APIHandler {
	return &APIHandler{parser: p, transactionStorage: ts}
}

// HTTP handlers
func (h *APIHandler) getCurrentBlock(w http.ResponseWriter, r *http.Request) {
	currentBlock := h.parser.GetCurrentBlockNumber()
	h.sendSuccess(w, currentBlock)
}

func (h *APIHandler) subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Address == "" {
		h.sendError(w, "Address is required", http.StatusBadRequest)
		return
	}

	h.parser.Subscribe(req.Address)
	h.sendSuccess(w, map[string]bool{"subscribed": true})
}

func (h *APIHandler) getTransactions(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		h.sendError(w, "Address parameter is required", http.StatusBadRequest)
		return
	}

	if !h.parser.subscribers.Exists(address) {
		h.sendError(w, "address is not in subscriptions", http.StatusBadRequest)
		return
	}

	transactions, err := h.transactionStorage.GetTransactionsByAddress(address)
	if err != nil {
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendSuccess(w, transactions)
}

func (h *APIHandler) sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

func (h *APIHandler) sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

type TransactionStorage interface {
	Put(t Transaction) error
	GetTransactionsByAddress(address string) ([]Transaction, error)
}

type RAMTransactionStorage struct {
	mu           *sync.RWMutex
	transactions []Transaction
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

type Config struct {
	EthURL                  string
	BlockProcessingInterval time.Duration
	EthAPITimeout           time.Duration
}

type SubscribersStorage interface {
	Put(address string) error
	Exists(address string) bool
}

type RAMSubscribersStorage struct {
	mu        *sync.RWMutex
	addresses map[string]struct{}
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

type EthParser struct {
	conf               Config
	httpClient         *http.Client
	currentBlockNumber uint64
	subscribers        SubscribersStorage
	transactionStorage TransactionStorage
}

func NewEthParser(conf Config, ts TransactionStorage, ss SubscribersStorage) *EthParser {
	return &EthParser{
		conf:               conf,
		httpClient:         &http.Client{Timeout: conf.EthAPITimeout},
		subscribers:        ss,
		transactionStorage: ts,
	}
}

func (p *EthParser) Subscribe(address string) {
	p.subscribers.Put(strings.ToLower(address))
}

func (p *EthParser) GetCurrentBlockNumber() uint64 {
	return atomic.LoadUint64(&p.currentBlockNumber)
}

func (p *EthParser) setCurrentBlockNumber(v uint64) {
	atomic.StoreUint64(&p.currentBlockNumber, v)
}

func (p *EthParser) initCurrentBlockNumber(ctx context.Context) error {
	n, err := p.getLatestBlockNumber(ctx)
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

func (p *EthParser) start(ctx context.Context) context.CancelFunc {
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

	return cancel
}

func (p *EthParser) processLatestBlock(ctx context.Context) error {
	latestBlockNum := p.GetCurrentBlockNumber()
	latestBlockNum++
	numHex := fmt.Sprintf("0x%x", latestBlockNum)
	log.Printf("getting transactions for block %v\n", latestBlockNum)
	transactions, err := p.getBlockTransactions(ctx, numHex)
	if err != nil {
		return err
	}

	for _, t := range transactions {
		if p.subscribers.Exists(t.To) || p.subscribers.Exists(t.From) {
			if err := p.transactionStorage.Put(t); err != nil {
				return fmt.Errorf("couldn't store transaction: %w", err)
			}
		}
	}

	p.setCurrentBlockNumber(latestBlockNum)

	return nil
}

type BlockResp struct {
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	Hash  string `json:"hash"`
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `value:"hash"`
}

func (p *EthParser) getBlockTransactions(ctx context.Context, blockNum string) ([]Transaction, error) {
	resp, err := p.makeJSONRPCRequest(ctx, "eth_getBlockByNumber", []any{blockNum, true})
	if err != nil {
		return nil, fmt.Errorf("couldn't get block by number: %w", err)
	}

	var res BlockResp
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, fmt.Errorf("couldn't unmarshal block response result: %v", err)
	}

	return res.Transactions, nil
}

func (p *EthParser) getLatestBlockNumber(ctx context.Context) (string, error) {
	resp, err := p.makeJSONRPCRequest(ctx, "eth_blockNumber", nil)
	if err != nil {
		return "", fmt.Errorf("couldn't get the latest block number: %w", err)
	}

	var num string
	if err := json.Unmarshal(resp, &num); err != nil {
		return "", fmt.Errorf("RPC response is not valid: %#v", resp)
	}

	return num, nil
}

type JSONRPCRequest struct {
	JsonRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type JSONRPCResponse struct {
	JsonRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	ID      int             `json:"id"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *EthParser) makeJSONRPCRequest(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	request := JSONRPCRequest{
		JsonRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.conf.EthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("couldn't create http request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("couldn't read response body: %w", err)
	}

	var response JSONRPCResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", response.Error.Message)
	}

	return response.Result, nil
}
