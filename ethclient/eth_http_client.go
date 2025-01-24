package ethclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type BlockResp struct {
	Transactions []Transaction `json:"transactions"`
}

type EthAPIClient struct {
	config     Config
	httpClient *http.Client
}

type Config struct {
	EthURL  string
	TimeOut time.Duration
}

func NewEthAPIClient(c Config) *EthAPIClient {
	return &EthAPIClient{
		config: c,
		httpClient: &http.Client{
			Timeout: c.TimeOut,
		},
	}
}

func (c *EthAPIClient) GetBlockTransactions(ctx context.Context, blockNum string) ([]Transaction, error) {
	resp, err := c.makeJSONRPCRequest(ctx, "eth_getBlockByNumber", []any{blockNum, true})
	if err != nil {
		return nil, fmt.Errorf("couldn't get block by number: %w", err)
	}

	var res BlockResp
	if err := json.Unmarshal(resp, &res); err != nil {
		return nil, fmt.Errorf("couldn't unmarshal block response result: %v", err)
	}

	return res.Transactions, nil
}

func (c *EthAPIClient) GetLatestBlockNumber(ctx context.Context) (string, error) {
	resp, err := c.makeJSONRPCRequest(ctx, "eth_blockNumber", nil)
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

func (c *EthAPIClient) makeJSONRPCRequest(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.EthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("couldn't create http request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
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
