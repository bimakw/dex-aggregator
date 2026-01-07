package entities

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Hop represents a single swap step in a route
type Hop struct {
	Pair     Pair           `json:"pair"`
	TokenIn  common.Address `json:"tokenIn"`
	TokenOut common.Address `json:"tokenOut"`
}

// Route represents a swap path from tokenIn to tokenOut
type Route struct {
	Hops        []Hop    `json:"hops"`
	TokenIn     Token    `json:"tokenIn"`
	TokenOut    Token    `json:"tokenOut"`
	AmountIn    *big.Int `json:"amountIn"`
	AmountOut   *big.Int `json:"amountOut"`
	PriceImpact *big.Int `json:"priceImpact"` // In basis points (e.g., 50 = 0.5%)
	GasEstimate uint64   `json:"gasEstimate"`
}

// Quote represents the result of a price quote request
type Quote struct {
	TokenIn     Token              `json:"tokenIn"`
	TokenOut    Token              `json:"tokenOut"`
	AmountIn    *big.Int           `json:"amountIn"`
	AmountOut   *big.Int           `json:"amountOut"`
	BestRoute   *Route             `json:"bestRoute"`
	PriceImpact *big.Int           `json:"priceImpact"`
	GasEstimate uint64             `json:"gasEstimate"`
	Sources     map[DEXType]string `json:"sources"` // Price quotes from each DEX
}

// CalculateAmountOut calculates the final output amount for the entire route
func (r *Route) CalculateAmountOut() *big.Int {
	if len(r.Hops) == 0 || r.AmountIn == nil {
		return big.NewInt(0)
	}

	currentAmount := new(big.Int).Set(r.AmountIn)
	for _, hop := range r.Hops {
		currentAmount = hop.Pair.GetAmountOut(currentAmount, hop.TokenIn)
		if currentAmount.Sign() <= 0 {
			return big.NewInt(0)
		}
	}

	return currentAmount
}

// CalculatePriceImpact calculates the price impact in basis points
// Price impact = (spotPrice - executionPrice) / spotPrice * 10000
func (r *Route) CalculatePriceImpact() *big.Int {
	if len(r.Hops) == 0 || r.AmountIn == nil || r.AmountIn.Sign() == 0 {
		return big.NewInt(0)
	}

	// Calculate spot price (price for infinitesimally small trade)
	spotAmount := r.calculateSpotAmount()
	if spotAmount.Sign() == 0 {
		return big.NewInt(0)
	}

	actualAmount := r.CalculateAmountOut()
	if actualAmount.Sign() == 0 {
		return big.NewInt(10000) // 100% price impact if no output
	}

	// priceImpact = (spotAmount - actualAmount) / spotAmount * 10000
	diff := new(big.Int).Sub(spotAmount, actualAmount)
	if diff.Sign() <= 0 {
		return big.NewInt(0)
	}

	impactScaled := new(big.Int).Mul(diff, big.NewInt(10000))
	return new(big.Int).Div(impactScaled, spotAmount)
}

// calculateSpotAmount calculates the theoretical output at spot price (no slippage)
func (r *Route) calculateSpotAmount() *big.Int {
	if len(r.Hops) == 0 || r.AmountIn == nil {
		return big.NewInt(0)
	}

	// For spot price calculation, we simulate with a very small amount
	// and scale up proportionally
	testAmount := big.NewInt(1e15) // 0.001 tokens with 18 decimals
	testOutput := new(big.Int).Set(testAmount)

	for _, hop := range r.Hops {
		testOutput = hop.Pair.GetAmountOut(testOutput, hop.TokenIn)
		if testOutput.Sign() <= 0 {
			return big.NewInt(0)
		}
	}

	// Scale up: spotAmount = (amountIn * testOutput) / testAmount
	scaled := new(big.Int).Mul(r.AmountIn, testOutput)
	return new(big.Int).Div(scaled, testAmount)
}
