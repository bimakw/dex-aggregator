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

// Uniswap V3 contract addresses (Ethereum Mainnet)
var (
	UniswapV3FactoryAddress = common.HexToAddress("0x1F98431c8aD98523631AE4a59f267346ea31F984")
	UniswapV3QuoterV2       = common.HexToAddress("0x61fFE014bA17989E743c5F6cB21bF9697530B21e")
)

// Uniswap V3 fee tiers in hundredths of a bip (1 = 0.0001%)
var V3FeeTiers = []uint32{
	100,   // 0.01%
	500,   // 0.05%
	3000,  // 0.30%
	10000, // 1.00%
}

var (
	// getPool(address,address,uint24) returns (address)
	getPoolSelector = common.Hex2Bytes("1698ee82")
	// quoteExactInputSingle((address,address,uint256,uint24,uint160)) returns (uint256,uint160,uint32,uint256)
	quoteExactInputSingleSelector = common.Hex2Bytes("c6a5026a")
)

// UniswapV3Client fetches price data from Uniswap V3
type UniswapV3Client struct {
	ethClient *ethclient.Client
	factory   common.Address
	quoter    common.Address
}

func NewUniswapV3Client(ethClient *ethclient.Client) *UniswapV3Client {
	return &UniswapV3Client{
		ethClient: ethClient,
		factory:   UniswapV3FactoryAddress,
		quoter:    UniswapV3QuoterV2,
	}
}

func (c *UniswapV3Client) GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	token0, token1 := sortTokens(tokenA, tokenB)

	for _, fee := range V3FeeTiers {
		poolAddr, err := c.getPool(ctx, token0, token1, fee)
		if err != nil {
			continue
		}
		if poolAddr != ethclient.ZeroAddress {
			return poolAddr, nil
		}
	}

	return common.Address{}, fmt.Errorf("no V3 pool found for token pair")
}

// getPool calls factory.getPool to get pool address for specific fee tier
func (c *UniswapV3Client) getPool(ctx context.Context, token0, token1 common.Address, fee uint32) (common.Address, error) {
	// Encode: getPool(address,address,uint24)
	data := make([]byte, 100)
	copy(data[0:4], getPoolSelector)
	copy(data[16:36], token0.Bytes())
	copy(data[48:68], token1.Bytes())
	// fee is uint24, put in last 3 bytes of the 32-byte slot
	feeBig := big.NewInt(int64(fee))
	feeBytes := feeBig.Bytes()
	copy(data[100-len(feeBytes):100], feeBytes)

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &c.factory,
		Data: data,
	})
	if err != nil {
		return common.Address{}, err
	}

	if len(result) < 32 {
		return common.Address{}, fmt.Errorf("invalid response length")
	}

	return common.BytesToAddress(result[12:32]), nil
}

func (c *UniswapV3Client) GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error) {
	token0, token1 := tokenA, tokenB
	if tokenA.Address.Hex() > tokenB.Address.Hex() {
		token0, token1 = tokenB, tokenA
	}

	var bestPool common.Address
	var bestFee uint32

	for _, fee := range V3FeeTiers {
		poolAddr, err := c.getPool(ctx, token0.Address, token1.Address, fee)
		if err != nil || poolAddr == ethclient.ZeroAddress {
			continue
		}
		// Use first found pool (typically 0.3% has most liquidity)
		bestPool = poolAddr
		bestFee = fee
		break
	}

	if bestPool == ethclient.ZeroAddress {
		return nil, fmt.Errorf("no V3 pool found for token pair")
	}

	// V3 doesn't use reserves like V2, but we create a Pair struct for compatibility
	return &entities.Pair{
		Address:   bestPool,
		Token0:    token0,
		Token1:    token1,
		Reserve0:  big.NewInt(0), // V3 uses concentrated liquidity, not reserves
		Reserve1:  big.NewInt(0),
		DEX:       entities.DEXUniswapV3,
		Fee:       uint64(bestFee), // Fee in hundredths of a bip
		UpdatedAt: time.Now().Unix(),
	}, nil
}

func (c *UniswapV3Client) GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return big.NewInt(0), nil
	}

	var bestAmountOut *big.Int

	for _, fee := range V3FeeTiers {
		amountOut, err := c.quoteExactInputSingle(ctx, tokenIn.Address, tokenOut.Address, amountIn, fee)
		if err != nil {
			continue
		}

		if bestAmountOut == nil || amountOut.Cmp(bestAmountOut) > 0 {
			bestAmountOut = amountOut
		}
	}

	if bestAmountOut == nil {
		return nil, fmt.Errorf("failed to get quote from any V3 pool")
	}

	return bestAmountOut, nil
}

// quoteExactInputSingle calls QuoterV2 to get exact output amount
// Struct params: (tokenIn, tokenOut, amountIn, fee, sqrtPriceLimitX96)
func (c *UniswapV3Client) quoteExactInputSingle(ctx context.Context, tokenIn, tokenOut common.Address, amountIn *big.Int, fee uint32) (*big.Int, error) {
	// QuoteExactInputSingleParams struct:
	// - tokenIn (address): 32 bytes
	// - tokenOut (address): 32 bytes
	// - amountIn (uint256): 32 bytes
	// - fee (uint24): 32 bytes
	// - sqrtPriceLimitX96 (uint160): 32 bytes

	data := make([]byte, 4+32*5) // selector + 5 params
	copy(data[0:4], quoteExactInputSingleSelector)

	// tokenIn at offset 4
	copy(data[4+12:4+32], tokenIn.Bytes())

	// tokenOut at offset 36
	copy(data[36+12:36+32], tokenOut.Bytes())

	// amountIn at offset 68
	amountInBytes := amountIn.Bytes()
	copy(data[68+32-len(amountInBytes):68+32], amountInBytes)

	// fee at offset 100
	feeBig := big.NewInt(int64(fee))
	feeBytes := feeBig.Bytes()
	copy(data[100+32-len(feeBytes):100+32], feeBytes)

	// sqrtPriceLimitX96 at offset 132 - set to 0 for no limit

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &c.quoter,
		Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf("quoter call failed: %w", err)
	}

	// Response: (amountOut uint256, sqrtPriceX96After uint160, initializedTicksCrossed uint32, gasEstimate uint256)
	if len(result) < 32 {
		return nil, fmt.Errorf("invalid quoter response length: %d", len(result))
	}

	amountOut := new(big.Int).SetBytes(result[0:32])
	return amountOut, nil
}

// DEXType returns the DEX type identifier
func (c *UniswapV3Client) DEXType() entities.DEXType {
	return entities.DEXUniswapV3
}
