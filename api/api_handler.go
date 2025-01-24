package api

import (
	"encoding/json"
	"net/http"
	"tw_eth_parser/storage"
)

type SubscribeRequest struct {
	Address string `json:"address"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type EthParserService interface {
	Subscribe(address string)
	GetCurrentBlockNumber() uint64
	Exists(address string) bool
	GetTransactionsByAddress(address string) ([]storage.Transaction, error)
}

type APIHandler struct {
	parser EthParserService
}

func NewAPIHandler(p EthParserService) *APIHandler {
	return &APIHandler{parser: p}
}

// HTTP handlers
func (h *APIHandler) GetCurrentBlock(w http.ResponseWriter, r *http.Request) {
	currentBlock := h.parser.GetCurrentBlockNumber()
	h.sendSuccess(w, currentBlock)
}

func (h *APIHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
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

func (h *APIHandler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		h.sendError(w, "Address parameter is required", http.StatusBadRequest)
		return
	}

	if !h.parser.Exists(address) {
		h.sendError(w, "address is not in subscriptions", http.StatusBadRequest)
		return
	}

	transactions, err := h.parser.GetTransactionsByAddress(address)
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
