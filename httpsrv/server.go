package httpsrv

import (
	"log"
	"net/http"
	"strconv"
	"tw_eth_parser/api"
)

type HTTPServer struct {
	ethAPIHandler *api.APIHandler
	port          int
}

func NewHTTPServer(h *api.APIHandler, port int) *HTTPServer {
	return &HTTPServer{
		ethAPIHandler: h,
		port:          port,
	}
}

func (s *HTTPServer) Start() error {
	// API routes
	http.HandleFunc("GET /api/block/current", s.ethAPIHandler.GetCurrentBlock)
	http.HandleFunc("POST /api/subscribe", s.ethAPIHandler.Subscribe)
	http.HandleFunc("GET /api/transactions", s.ethAPIHandler.GetTransactions)

	// Start server
	port := ":" + strconv.Itoa(s.port)
	log.Printf("Server starting on port %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		return err
	}

	return nil
}
