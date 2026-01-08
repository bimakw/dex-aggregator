package dex

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	ethclient "github.com/bimakw/dex-aggregator/internal/infrastructure/ethereum"
)

// Curve pool function selectors
var (
	// get_dy(int128 i, int128 j, uint256 dx) returns (uint256)
	getDySelector = common.Hex2Bytes("5e0d443f")
	// coins(uint256) returns (address)
	coinsSelector = common.Hex2Bytes("c6610657")
	// balances(uint256) returns (uint256)
	balancesSelector = common.Hex2Bytes("4903b0d1")
	// fee() returns (uint256) - fee in 1e10 format
	feeSelector = common.Hex2Bytes("ddca3f43")
)

// Curve stablecoin pool addresses (Ethereum mainnet)
var (
	// 3pool (DAI/USDC/USDT)
	Curve3PoolAddress = common.HexToAddress("0xbEbc44782C7dB0a1A60Cb6fe97d0b483032FF1C7")
	// stETH/ETH pool
	CurveStETHAddress = common.HexToAddress("0xDC24316b9AE028F1497c275EB9192a3Ea0f67022")
)

// CurvePool represents a Curve pool configuration
type CurvePool struct {
	Address common.Address
	Coins   []common.Address
	Name    string
}

// Known Curve pools
var curvePools = []CurvePool{
	{
		Address: Curve3PoolAddress,
		Coins: []common.Address{
			entities.DAI.Address,
			entities.USDC.Address,
			entities.USDT.Address,
		},
		Name: "3pool",
	},
}

// CurveClient fetches price data from Curve Finance pools
type CurveClient struct {
	ethClient *ethclient.Client
	pools     []CurvePool
}

// NewCurveClient creates a new Curve Finance client
func NewCurveClient(ethClient *ethclient.Client) *CurveClient {
	return &CurveClient{
		ethClient: ethClient,
		pools:     curvePools,
	}
}

// GetPairAddress returns the pool address for two tokens
func (c *CurveClient) GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	for _, pool := range c.pools {
		hasA, hasB := false, false
		for _, coin := range pool.Coins {
			if coin == tokenA {
				hasA = true
			}
			if coin == tokenB {
				hasB = true
			}
		}
		if hasA && hasB {
			return pool.Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("no Curve pool found for token pair")
}

// GetPairByTokens fetches pool data by token addresses
func (c *CurveClient) GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error) {
	poolAddress, err := c.GetPairAddress(ctx, tokenA.Address, tokenB.Address)
	if err != nil {
		return nil, err
	}

	// Find the pool configuration
	var pool *CurvePool
	for i := range c.pools {
		if c.pools[i].Address == poolAddress {
			pool = &c.pools[i]
			break
		}
	}
	if pool == nil {
		return nil, fmt.Errorf("pool configuration not found")
	}

	// Get token indices in the pool
	idxA, idxB := -1, -1
	for i, coin := range pool.Coins {
		if coin == tokenA.Address {
			idxA = i
		}
		if coin == tokenB.Address {
			idxB = i
		}
	}
	if idxA == -1 || idxB == -1 {
		return nil, fmt.Errorf("token not found in pool")
	}

	// Get balances for both tokens
	balanceA, err := c.getBalance(ctx, poolAddress, idxA)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance A: %w", err)
	}
	balanceB, err := c.getBalance(ctx, poolAddress, idxB)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance B: %w", err)
	}

	// Get fee (Curve uses 1e10 format, we want basis points)
	fee, err := c.getFee(ctx, poolAddress)
	if err != nil {
		// Default to 0.04% for stablecoin pools
		fee = 4
	}

	// Sort tokens for consistent ordering
	var token0, token1 entities.Token
	var reserve0, reserve1 *big.Int
	if tokenA.Address.Hex() < tokenB.Address.Hex() {
		token0, token1 = tokenA, tokenB
		reserve0, reserve1 = balanceA, balanceB
	} else {
		token0, token1 = tokenB, tokenA
		reserve0, reserve1 = balanceB, balanceA
	}

	return &entities.Pair{
		Address:   poolAddress,
		Token0:    token0,
		Token1:    token1,
		Reserve0:  reserve0,
		Reserve1:  reserve1,
		DEX:       entities.DEXCurve,
		Fee:       fee,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// GetAmountOut calculates the output amount for a swap using get_dy
func (c *CurveClient) GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error) {
	poolAddress, err := c.GetPairAddress(ctx, tokenIn.Address, tokenOut.Address)
	if err != nil {
		return nil, err
	}

	// Find pool and token indices
	var pool *CurvePool
	for i := range c.pools {
		if c.pools[i].Address == poolAddress {
			pool = &c.pools[i]
			break
		}
	}
	if pool == nil {
		return nil, fmt.Errorf("pool not found")
	}

	idxIn, idxOut := -1, -1
	for i, coin := range pool.Coins {
		if coin == tokenIn.Address {
			idxIn = i
		}
		if coin == tokenOut.Address {
			idxOut = i
		}
	}
	if idxIn == -1 || idxOut == -1 {
		return nil, fmt.Errorf("token not found in pool")
	}

	// Call get_dy(i, j, dx)
	data := make([]byte, 100)
	copy(data[0:4], getDySelector)
	// i (int128) - padded to 32 bytes
	big.NewInt(int64(idxIn)).FillBytes(data[4:36])
	// j (int128) - padded to 32 bytes
	big.NewInt(int64(idxOut)).FillBytes(data[36:68])
	// dx (uint256) - padded to 32 bytes
	amountIn.FillBytes(data[68:100])

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &poolAddress,
		Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf("get_dy call failed: %w", err)
	}

	if len(result) < 32 {
		return nil, fmt.Errorf("invalid get_dy response")
	}

	return new(big.Int).SetBytes(result[0:32]), nil
}

// DEXType returns the DEX type
func (c *CurveClient) DEXType() entities.DEXType {
	return entities.DEXCurve
}

// getBalance fetches the balance of a token at a given index
func (c *CurveClient) getBalance(ctx context.Context, pool common.Address, idx int) (*big.Int, error) {
	data := make([]byte, 36)
	copy(data[0:4], balancesSelector)
	big.NewInt(int64(idx)).FillBytes(data[4:36])

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &pool,
		Data: data,
	})
	if err != nil {
		return nil, err
	}

	if len(result) < 32 {
		return nil, fmt.Errorf("invalid balance response")
	}

	return new(big.Int).SetBytes(result[0:32]), nil
}

// getFee fetches the pool fee and converts to basis points
func (c *CurveClient) getFee(ctx context.Context, pool common.Address) (uint64, error) {
	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &pool,
		Data: feeSelector,
	})
	if err != nil {
		return 0, err
	}

	if len(result) < 32 {
		return 0, fmt.Errorf("invalid fee response")
	}

	// Curve fee is in 1e10 format (e.g., 4000000 = 0.04%)
	// Convert to basis points (1 bp = 0.01%)
	fee := new(big.Int).SetBytes(result[0:32])
	// fee_bps = fee / 1e6
	feeBps := new(big.Int).Div(fee, big.NewInt(1e6))
	return feeBps.Uint64(), nil
}
