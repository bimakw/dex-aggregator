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

// Balancer V2 contract addresses (Ethereum mainnet)
var (
	BalancerVaultAddress = common.HexToAddress("0xBA12222222228d8Ba445958a75a0704d566BF2C8")
)

var (
	// getPoolTokens(bytes32 poolId) returns (address[] tokens, uint256[] balances, uint256 lastChangeBlock)
	getPoolTokensSelector = common.Hex2Bytes("f94d4668")
	// queryBatchSwap(uint8 kind, SwapStep[] swaps, address[] assets, FundManagement funds)
)

type BalancerPool struct {
	PoolID  [32]byte
	Address common.Address
	Tokens  []common.Address
	Weights []uint64 // Weights in basis points (e.g., 5000 = 50%)
	SwapFee uint64   // Fee in basis points
	Name    string
}

var balancerPools = []BalancerPool{
	{
		// WETH/DAI 60/40 pool
		PoolID:  hexToBytes32("0x0b09dea16768f0799065c475be02919503cb2a3500020000000000000000001a"),
		Address: common.HexToAddress("0x0b09deA16768f0799065C475bE02919503cB2a35"),
		Tokens: []common.Address{
			entities.WETH.Address,
			entities.DAI.Address,
		},
		Weights: []uint64{6000, 4000}, // 60% WETH, 40% DAI
		SwapFee: 30,                   // 0.3%
		Name:    "WETH/DAI 60/40",
	},
	{
		// WETH/USDC 50/50 pool (example)
		PoolID:  hexToBytes32("0x96646936b91d6b9d7d0c47c496afbf3d6ec7b6f8000200000000000000000019"),
		Address: common.HexToAddress("0x96646936b91d6B9D7D0c47C496AfBF3D6ec7B6f8"),
		Tokens: []common.Address{
			entities.WETH.Address,
			entities.USDC.Address,
		},
		Weights: []uint64{5000, 5000}, // 50% WETH, 50% USDC
		SwapFee: 30,                   // 0.3%
		Name:    "WETH/USDC 50/50",
	},
}

type BalancerClient struct {
	ethClient *ethclient.Client
	vault     common.Address
	pools     []BalancerPool
}

func NewBalancerClient(ethClient *ethclient.Client) *BalancerClient {
	return &BalancerClient{
		ethClient: ethClient,
		vault:     BalancerVaultAddress,
		pools:     balancerPools,
	}
}

func (c *BalancerClient) GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	for _, pool := range c.pools {
		hasA, hasB := false, false
		for _, token := range pool.Tokens {
			if token == tokenA {
				hasA = true
			}
			if token == tokenB {
				hasB = true
			}
		}
		if hasA && hasB {
			return pool.Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("no Balancer pool found for token pair")
}

func (c *BalancerClient) GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error) {
	var pool *BalancerPool
	for i := range c.pools {
		hasA, hasB := false, false
		for _, token := range c.pools[i].Tokens {
			if token == tokenA.Address {
				hasA = true
			}
			if token == tokenB.Address {
				hasB = true
			}
		}
		if hasA && hasB {
			pool = &c.pools[i]
			break
		}
	}
	if pool == nil {
		return nil, fmt.Errorf("no Balancer pool found for token pair")
	}

	balances, err := c.getPoolTokens(ctx, pool.PoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool tokens: %w", err)
	}

	idxA, idxB := -1, -1
	for i, token := range pool.Tokens {
		if token == tokenA.Address {
			idxA = i
		}
		if token == tokenB.Address {
			idxB = i
		}
	}
	if idxA == -1 || idxB == -1 || idxA >= len(balances) || idxB >= len(balances) {
		return nil, fmt.Errorf("token not found in pool")
	}

	var token0, token1 entities.Token
	var reserve0, reserve1 *big.Int
	if tokenA.Address.Hex() < tokenB.Address.Hex() {
		token0, token1 = tokenA, tokenB
		reserve0, reserve1 = balances[idxA], balances[idxB]
	} else {
		token0, token1 = tokenB, tokenA
		reserve0, reserve1 = balances[idxB], balances[idxA]
	}

	return &entities.Pair{
		Address:   pool.Address,
		Token0:    token0,
		Token1:    token1,
		Reserve0:  reserve0,
		Reserve1:  reserve1,
		DEX:       entities.DEXBalancer,
		Fee:       pool.SwapFee,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// Uses the weighted math formula: outAmount = balanceOut * (1 - (balanceIn / (balanceIn + amountIn))^(weightIn/weightOut))
func (c *BalancerClient) GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error) {
	var pool *BalancerPool
	for i := range c.pools {
		hasIn, hasOut := false, false
		for _, token := range c.pools[i].Tokens {
			if token == tokenIn.Address {
				hasIn = true
			}
			if token == tokenOut.Address {
				hasOut = true
			}
		}
		if hasIn && hasOut {
			pool = &c.pools[i]
			break
		}
	}
	if pool == nil {
		return nil, fmt.Errorf("no Balancer pool found")
	}

	balances, err := c.getPoolTokens(ctx, pool.PoolID)
	if err != nil {
		return nil, err
	}

	idxIn, idxOut := -1, -1
	for i, token := range pool.Tokens {
		if token == tokenIn.Address {
			idxIn = i
		}
		if token == tokenOut.Address {
			idxOut = i
		}
	}
	if idxIn == -1 || idxOut == -1 {
		return nil, fmt.Errorf("token not found in pool")
	}

	balanceIn := balances[idxIn]
	balanceOut := balances[idxOut]
	weightIn := pool.Weights[idxIn]
	weightOut := pool.Weights[idxOut]

	// For weighted pools: amountOut = balanceOut * (1 - (balanceIn / (balanceIn + amountIn * (1 - fee)))^(wIn/wOut))
	// Simplified for equal weights: amountOut ≈ balanceOut * amountIn * (1 - fee) / (balanceIn + amountIn * (1 - fee))
	return c.calcOutGivenIn(balanceIn, balanceOut, amountIn, weightIn, weightOut, pool.SwapFee), nil
}

// calcOutGivenIn calculates output amount using weighted math
func (c *BalancerClient) calcOutGivenIn(balanceIn, balanceOut, amountIn *big.Int, weightIn, weightOut, feeBps uint64) *big.Int {
	feeMultiplier := big.NewInt(10000 - int64(feeBps))
	amountInAfterFee := new(big.Int).Mul(amountIn, feeMultiplier)
	amountInAfterFee.Div(amountInAfterFee, big.NewInt(10000))

	if weightIn == weightOut {
		// amountOut = balanceOut * amountInAfterFee / (balanceIn + amountInAfterFee)
		numerator := new(big.Int).Mul(balanceOut, amountInAfterFee)
		denominator := new(big.Int).Add(balanceIn, amountInAfterFee)
		return new(big.Int).Div(numerator, denominator)
	}

	// amountOut ≈ balanceOut * (amountInAfterFee / balanceIn) * (weightIn / weightOut)
	precision := big.NewInt(1e18)

	// ratio = amountInAfterFee * precision / balanceIn
	ratio := new(big.Int).Mul(amountInAfterFee, precision)
	ratio.Div(ratio, balanceIn)

	// weightRatio = weightIn * precision / weightOut
	weightRatio := new(big.Int).Mul(big.NewInt(int64(weightIn)), precision)
	weightRatio.Div(weightRatio, big.NewInt(int64(weightOut)))

	// amountOut = balanceOut * ratio * weightRatio / precision^2
	amountOut := new(big.Int).Mul(balanceOut, ratio)
	amountOut.Mul(amountOut, weightRatio)
	amountOut.Div(amountOut, precision)
	amountOut.Div(amountOut, precision)

	return amountOut
}

// DEXType returns the DEX type
func (c *BalancerClient) DEXType() entities.DEXType {
	return entities.DEXBalancer
}

// getPoolTokens fetches token balances from the vault
func (c *BalancerClient) getPoolTokens(ctx context.Context, poolID [32]byte) ([]*big.Int, error) {
	// Encode getPoolTokens(poolId)
	data := make([]byte, 36)
	copy(data[0:4], getPoolTokensSelector)
	copy(data[4:36], poolID[:])

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &c.vault,
		Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf("getPoolTokens call failed: %w", err)
	}

	// Parse result - returns (address[] tokens, uint256[] balances, uint256 lastChangeBlock)
	if len(result) < 192 {
		return nil, fmt.Errorf("invalid getPoolTokens response length")
	}

	// Skip to balances array (offset at position 32-64)
	balancesOffset := new(big.Int).SetBytes(result[32:64]).Uint64()
	if balancesOffset+32 > uint64(len(result)) {
		return nil, fmt.Errorf("invalid balances offset")
	}

	balancesLen := new(big.Int).SetBytes(result[balancesOffset : balancesOffset+32]).Uint64()

	balances := make([]*big.Int, balancesLen)
	for i := uint64(0); i < balancesLen; i++ {
		start := balancesOffset + 32 + i*32
		if start+32 > uint64(len(result)) {
			return nil, fmt.Errorf("invalid balance data")
		}
		balances[i] = new(big.Int).SetBytes(result[start : start+32])
	}

	return balances, nil
}

// hexToBytes32 converts a hex string to [32]byte
func hexToBytes32(hex string) [32]byte {
	var result [32]byte
	bytes := common.FromHex(hex)
	copy(result[:], bytes)
	return result
}
