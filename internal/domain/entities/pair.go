package entities

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// DEXType represents the type of decentralized exchange
type DEXType string

const (
	DEXUniswapV2 DEXType = "uniswap_v2"
	DEXUniswapV3 DEXType = "uniswap_v3"
	DEXSushiswap DEXType = "sushiswap"
	DEXCurve     DEXType = "curve"
	DEXBalancer  DEXType = "balancer"
)

// Pair represents a liquidity pair on a DEX
type Pair struct {
	Address   common.Address `json:"address"`
	Token0    Token          `json:"token0"`
	Token1    Token          `json:"token1"`
	Reserve0  *big.Int       `json:"reserve0"`
	Reserve1  *big.Int       `json:"reserve1"`
	DEX       DEXType        `json:"dex"`
	Fee       uint64         `json:"fee"` // Fee in basis points (e.g., 30 = 0.3%)
	UpdatedAt int64          `json:"updatedAt"`
}

// GetSpotPrice calculates the spot price of token0 in terms of token1
func (p *Pair) GetSpotPrice() *big.Int {
	if p.Reserve0 == nil || p.Reserve1 == nil || p.Reserve0.Sign() == 0 {
		return big.NewInt(0)
	}

	precision := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	numerator := new(big.Int).Mul(p.Reserve1, precision)
	return new(big.Int).Div(numerator, p.Reserve0)
}

func (p *Pair) GetAmountOut(amountIn *big.Int, tokenIn common.Address) *big.Int {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return big.NewInt(0)
	}

	var reserveIn, reserveOut *big.Int
	if tokenIn == p.Token0.Address {
		reserveIn = p.Reserve0
		reserveOut = p.Reserve1
	} else {
		reserveIn = p.Reserve1
		reserveOut = p.Reserve0
	}

	if reserveIn == nil || reserveOut == nil || reserveIn.Sign() == 0 || reserveOut.Sign() == 0 {
		return big.NewInt(0)
	}

	// Apply fee (e.g., 0.3% fee means multiply by 997/1000)
	feeMultiplier := big.NewInt(10000 - int64(p.Fee))
	amountInWithFee := new(big.Int).Mul(amountIn, feeMultiplier)

	// numerator = amountInWithFee * reserveOut
	numerator := new(big.Int).Mul(amountInWithFee, reserveOut)

	// denominator = reserveIn * 10000 + amountInWithFee
	denominator := new(big.Int).Mul(reserveIn, big.NewInt(10000))
	denominator.Add(denominator, amountInWithFee)

	return new(big.Int).Div(numerator, denominator)
}
