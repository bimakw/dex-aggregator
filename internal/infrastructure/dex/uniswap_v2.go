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

// UniswapV2 ABI function signatures (keccak256 hash of function signature)
var (
	// getReserves() returns (uint112 reserve0, uint112 reserve1, uint32 blockTimestampLast)
	getReservesSelector = common.Hex2Bytes("0902f1ac")
	// token0() returns (address)
	token0Selector = common.Hex2Bytes("0dfe1681")
	// token1() returns (address)
	token1Selector = common.Hex2Bytes("d21220a7")
	// getPair(address,address) returns (address)
	getPairSelector = common.Hex2Bytes("e6a43905")
)

// UniswapV2Factory addresses
var (
	UniswapV2FactoryAddress = common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f")
	SushiswapFactoryAddress = common.HexToAddress("0xC0AEe478e3658e2610c5F7A4A2E1777cE9e4f2Ac")
)

// UniswapV2Client fetches pair data from Uniswap V2 compatible DEXes
type UniswapV2Client struct {
	ethClient *ethclient.Client
	factory   common.Address
	dexType   entities.DEXType
	fee       uint64 // Fee in basis points (30 = 0.3%)
}

// NewUniswapV2Client creates a new Uniswap V2 client
func NewUniswapV2Client(ethClient *ethclient.Client) *UniswapV2Client {
	return &UniswapV2Client{
		ethClient: ethClient,
		factory:   UniswapV2FactoryAddress,
		dexType:   entities.DEXUniswapV2,
		fee:       30, // 0.3% fee
	}
}

// NewSushiswapClient creates a new Sushiswap client (uses same interface as Uniswap V2)
func NewSushiswapClient(ethClient *ethclient.Client) *UniswapV2Client {
	return &UniswapV2Client{
		ethClient: ethClient,
		factory:   SushiswapFactoryAddress,
		dexType:   entities.DEXSushiswap,
		fee:       30, // 0.3% fee
	}
}

// GetPairAddress returns the pair address for two tokens
func (c *UniswapV2Client) GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	// Sort tokens (Uniswap V2 convention)
	token0, token1 := sortTokens(tokenA, tokenB)

	// Encode getPair(token0, token1)
	data := make([]byte, 68)
	copy(data[0:4], getPairSelector)
	copy(data[16:36], token0.Bytes())
	copy(data[48:68], token1.Bytes())

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &c.factory,
		Data: data,
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get pair address: %w", err)
	}

	if len(result) < 32 {
		return common.Address{}, fmt.Errorf("invalid response length")
	}

	pairAddress := common.BytesToAddress(result[12:32])
	return pairAddress, nil
}

// GetPair fetches pair data including reserves
func (c *UniswapV2Client) GetPair(ctx context.Context, pairAddress common.Address, token0, token1 entities.Token) (*entities.Pair, error) {
	reserves, err := c.getReserves(ctx, pairAddress)
	if err != nil {
		return nil, err
	}

	return &entities.Pair{
		Address:   pairAddress,
		Token0:    token0,
		Token1:    token1,
		Reserve0:  reserves[0],
		Reserve1:  reserves[1],
		DEX:       c.dexType,
		Fee:       c.fee,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// GetPairByTokens fetches pair data by token addresses
func (c *UniswapV2Client) GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error) {
	// Sort tokens to determine token0 and token1
	var token0, token1 entities.Token
	if tokenA.Address.Hex() < tokenB.Address.Hex() {
		token0, token1 = tokenA, tokenB
	} else {
		token0, token1 = tokenB, tokenA
	}

	pairAddress, err := c.GetPairAddress(ctx, token0.Address, token1.Address)
	if err != nil {
		return nil, err
	}

	if pairAddress == ethclient.ZeroAddress {
		return nil, fmt.Errorf("pair does not exist")
	}

	return c.GetPair(ctx, pairAddress, token0, token1)
}

// getReserves fetches reserves from a pair
func (c *UniswapV2Client) getReserves(ctx context.Context, pairAddress common.Address) ([2]*big.Int, error) {
	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &pairAddress,
		Data: getReservesSelector,
	})
	if err != nil {
		return [2]*big.Int{}, fmt.Errorf("failed to get reserves: %w", err)
	}

	if len(result) < 64 {
		return [2]*big.Int{}, fmt.Errorf("invalid reserves response length")
	}

	reserve0 := new(big.Int).SetBytes(result[0:32])
	reserve1 := new(big.Int).SetBytes(result[32:64])

	return [2]*big.Int{reserve0, reserve1}, nil
}

// GetAmountOut calculates the output amount for a swap
func (c *UniswapV2Client) GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error) {
	pair, err := c.GetPairByTokens(ctx, tokenIn, tokenOut)
	if err != nil {
		return nil, err
	}

	return pair.GetAmountOut(amountIn, tokenIn.Address), nil
}

// DEXType returns the DEX type
func (c *UniswapV2Client) DEXType() entities.DEXType {
	return c.dexType
}

// sortTokens sorts two addresses in ascending order (Uniswap V2 convention)
func sortTokens(tokenA, tokenB common.Address) (common.Address, common.Address) {
	if tokenA.Hex() < tokenB.Hex() {
		return tokenA, tokenB
	}
	return tokenB, tokenA
}
