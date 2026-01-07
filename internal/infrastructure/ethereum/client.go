package ethereum

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client wraps the go-ethereum client with additional functionality
type Client struct {
	client  *ethclient.Client
	rpcURL  string
	chainID *big.Int
	mu      sync.RWMutex
}

// NewClient creates a new Ethereum client
func NewClient(rpcURL string) (*Client, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}

	return &Client{
		client:  client,
		rpcURL:  rpcURL,
		chainID: chainID,
	}, nil
}

// Close closes the underlying client connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client.Close()
}

// ChainID returns the chain ID
func (c *Client) ChainID() *big.Int {
	return c.chainID
}

// CallContract executes a contract call
func (c *Client) CallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client.CallContract(ctx, msg, nil)
}

// BlockNumber returns the current block number
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client.BlockNumber(ctx)
}

// EstimateGas estimates the gas required for a transaction
func (c *Client) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client.EstimateGas(ctx, msg)
}

// SuggestGasPrice suggests a gas price based on recent blocks
func (c *Client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client.SuggestGasPrice(ctx)
}

// Multicall performs multiple contract calls in a single RPC request
// This is useful for fetching reserves from multiple pairs efficiently
func (c *Client) Multicall(ctx context.Context, calls []ethereum.CallMsg) ([][]byte, error) {
	results := make([][]byte, len(calls))
	errs := make([]error, len(calls))
	var wg sync.WaitGroup

	// Limit concurrent calls to prevent overwhelming the RPC
	semaphore := make(chan struct{}, 10)

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, msg ethereum.CallMsg) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := c.CallContract(ctx, msg)
			results[idx] = result
			errs[idx] = err
		}(i, call)
	}

	wg.Wait()

	// Return first error encountered
	for _, err := range errs {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Common Ethereum addresses
var (
	ZeroAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")
)
