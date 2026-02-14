package services

import (
	"context"
	"fmt"
	"math/big"
	"sort"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
)

// Default slippage tolerance in basis points (0.5%)
const DefaultSlippageBps = 50

// Price impact warning threshold in basis points (1%)
const PriceImpactWarningThreshold = 100

type RouterService struct {
	priceService *PriceService
}

func NewRouterService(priceService *PriceService) *RouterService {
	return &RouterService{
		priceService: priceService,
	}
}

func (s *RouterService) GetQuote(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int) (*entities.Quote, error) {
	prices, err := s.priceService.GetPrices(ctx, tokenIn, tokenOut, amountIn)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	var bestResult *PriceResult
	sources := make(map[entities.DEXType]string)

	for i := range prices {
		if prices[i].Error != nil {
			continue
		}
		if prices[i].AmountOut == nil || prices[i].AmountOut.Sign() <= 0 {
			continue
		}

		sources[prices[i].DEX] = prices[i].AmountOut.String()

		if bestResult == nil || prices[i].AmountOut.Cmp(bestResult.AmountOut) > 0 {
			bestResult = &prices[i]
		}
	}

	if bestResult == nil {
		return nil, fmt.Errorf("no valid routes found")
	}

	route := s.buildRoute(tokenIn, tokenOut, amountIn, bestResult)

	priceImpact := route.CalculatePriceImpact()

	return &entities.Quote{
		TokenIn:     tokenIn,
		TokenOut:    tokenOut,
		AmountIn:    amountIn,
		AmountOut:   bestResult.AmountOut,
		BestRoute:   route,
		PriceImpact: priceImpact,
		GasEstimate: estimateGas(route),
		Sources:     sources,
	}, nil
}

// buildRoute creates a Route from a price result
func (s *RouterService) buildRoute(tokenIn, tokenOut entities.Token, amountIn *big.Int, result *PriceResult) *entities.Route {
	hop := entities.Hop{
		Pair:     *result.Pair,
		TokenIn:  tokenIn.Address,
		TokenOut: tokenOut.Address,
	}

	return &entities.Route{
		Hops:        []entities.Hop{hop},
		TokenIn:     tokenIn,
		TokenOut:    tokenOut,
		AmountIn:    amountIn,
		AmountOut:   result.AmountOut,
		GasEstimate: estimateGas(nil), // Will be recalculated
	}
}

// estimateGas estimates gas for a route
func estimateGas(route *entities.Route) uint64 {
	if route == nil || len(route.Hops) == 0 {
		return 150000 // Default single swap estimate
	}

	baseGas := uint64(21000)
	gasPerHop := uint64(100000) // Approximate gas for a Uniswap V2 swap

	return baseGas + uint64(len(route.Hops))*gasPerHop
}

// GetMultiHopQuote finds the best route including multi-hop paths (Phase 3)
func (s *RouterService) GetMultiHopQuote(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int, intermediateTokens []entities.Token) (*entities.Quote, error) {
	directQuote, directErr := s.GetQuote(ctx, tokenIn, tokenOut, amountIn)

	var bestQuote *entities.Quote
	if directErr == nil {
		bestQuote = directQuote
	}

	for _, intermediate := range intermediateTokens {
		if intermediate.Address == tokenIn.Address || intermediate.Address == tokenOut.Address {
			continue
		}

		hop1Prices, err := s.priceService.GetPrices(ctx, tokenIn, intermediate, amountIn)
		if err != nil {
			continue
		}

		for _, hop1 := range hop1Prices {
			if hop1.Error != nil || hop1.AmountOut == nil || hop1.AmountOut.Sign() <= 0 {
				continue
			}

			hop2Prices, err := s.priceService.GetPrices(ctx, intermediate, tokenOut, hop1.AmountOut)
			if err != nil {
				continue
			}

			for _, hop2 := range hop2Prices {
				if hop2.Error != nil || hop2.AmountOut == nil || hop2.AmountOut.Sign() <= 0 {
					continue
				}

				if bestQuote == nil || hop2.AmountOut.Cmp(bestQuote.AmountOut) > 0 {
					route := &entities.Route{
						Hops: []entities.Hop{
							{Pair: *hop1.Pair, TokenIn: tokenIn.Address, TokenOut: intermediate.Address},
							{Pair: *hop2.Pair, TokenIn: intermediate.Address, TokenOut: tokenOut.Address},
						},
						TokenIn:     tokenIn,
						TokenOut:    tokenOut,
						AmountIn:    amountIn,
						AmountOut:   hop2.AmountOut,
						GasEstimate: estimateGas(nil),
					}
					route.GasEstimate = estimateGas(route)

					bestQuote = &entities.Quote{
						TokenIn:     tokenIn,
						TokenOut:    tokenOut,
						AmountIn:    amountIn,
						AmountOut:   hop2.AmountOut,
						BestRoute:   route,
						PriceImpact: route.CalculatePriceImpact(),
						GasEstimate: route.GasEstimate,
						Sources:     make(map[entities.DEXType]string),
					}
				}
			}
		}
	}

	if bestQuote == nil {
		return nil, fmt.Errorf("no valid routes found (direct or multi-hop)")
	}

	return bestQuote, nil
}

func (s *RouterService) GetSmartQuote(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int, slippageBps uint64) (*entities.Quote, error) {
	if slippageBps == 0 {
		slippageBps = DefaultSlippageBps
	}

	prices, err := s.priceService.GetPrices(ctx, tokenIn, tokenOut, amountIn)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	// Filter valid prices and sort by output amount (descending)
	validPrices := filterValidPrices(prices)
	if len(validPrices) == 0 {
		return nil, fmt.Errorf("no valid routes found")
	}

	var quote *entities.Quote
	if len(validPrices) >= 2 {
		splitQuote := s.trySplitOrder(tokenIn, tokenOut, amountIn, validPrices)
		if splitQuote != nil {
			quote = splitQuote
		}
	}

	if quote == nil {
		bestResult := &validPrices[0]
		route := s.buildRoute(tokenIn, tokenOut, amountIn, bestResult)

		sources := make(map[entities.DEXType]string)
		for _, p := range validPrices {
			sources[p.DEX] = p.AmountOut.String()
		}

		quote = &entities.Quote{
			TokenIn:     tokenIn,
			TokenOut:    tokenOut,
			AmountIn:    amountIn,
			AmountOut:   bestResult.AmountOut,
			BestRoute:   route,
			PriceImpact: route.CalculatePriceImpact(),
			GasEstimate: estimateGas(route),
			Sources:     sources,
		}
	}

	s.applySlippageProtection(quote, slippageBps)

	if quote.PriceImpact != nil && quote.PriceImpact.Cmp(big.NewInt(PriceImpactWarningThreshold)) > 0 {
		impactPct := float64(quote.PriceImpact.Int64()) / 100.0
		quote.PriceWarning = fmt.Sprintf("High price impact: %.2f%%", impactPct)
	}

	return quote, nil
}

// trySplitOrder attempts to split the order across multiple DEXes for better execution
func (s *RouterService) trySplitOrder(tokenIn, tokenOut entities.Token, amountIn *big.Int, prices []PriceResult) *entities.Quote {
	if len(prices) < 2 {
		return nil
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].AmountOut.Cmp(prices[j].AmountOut) > 0
	})

	bestSplitOutput := big.NewInt(0)
	var bestSplits []entities.SplitRoute
	bestGas := uint64(0)

	singleOutput := prices[0].AmountOut
	singleGas := estimateGas(nil)

	// Try splits: 50/50, 60/40, 70/30, 80/20
	splitRatios := [][]uint64{{50, 50}, {60, 40}, {70, 30}, {80, 20}}

	for _, ratio := range splitRatios {
		amount1 := new(big.Int).Mul(amountIn, big.NewInt(int64(ratio[0])))
		amount1.Div(amount1, big.NewInt(100))
		amount2 := new(big.Int).Sub(amountIn, amount1)

		output1 := prices[0].Pair.GetAmountOut(amount1, tokenIn.Address)
		output2 := prices[1].Pair.GetAmountOut(amount2, tokenIn.Address)

		totalOutput := new(big.Int).Add(output1, output2)
		totalGas := estimateGas(nil) * 2 // Two swaps

		// For simplicity, compare raw output (gas optimization would need ETH price)
		if totalOutput.Cmp(bestSplitOutput) > 0 {
			bestSplitOutput = totalOutput
			bestGas = totalGas

			route1 := &entities.Route{
				Hops: []entities.Hop{{
					Pair:     *prices[0].Pair,
					TokenIn:  tokenIn.Address,
					TokenOut: tokenOut.Address,
				}},
				TokenIn:     tokenIn,
				TokenOut:    tokenOut,
				AmountIn:    amount1,
				AmountOut:   output1,
				GasEstimate: estimateGas(nil),
			}
			route2 := &entities.Route{
				Hops: []entities.Hop{{
					Pair:     *prices[1].Pair,
					TokenIn:  tokenIn.Address,
					TokenOut: tokenOut.Address,
				}},
				TokenIn:     tokenIn,
				TokenOut:    tokenOut,
				AmountIn:    amount2,
				AmountOut:   output2,
				GasEstimate: estimateGas(nil),
			}

			bestSplits = []entities.SplitRoute{
				{Route: route1, Percentage: ratio[0], AmountIn: amount1, AmountOut: output1},
				{Route: route2, Percentage: ratio[1], AmountIn: amount2, AmountOut: output2},
			}
		}
	}

	if bestSplitOutput.Cmp(singleOutput) <= 0 {
		return nil
	}

	sources := make(map[entities.DEXType]string)
	for _, p := range prices {
		sources[p.DEX] = p.AmountOut.String()
	}

	bestRoute := s.buildRoute(tokenIn, tokenOut, amountIn, &prices[0])

	priceImpact := calculateSplitPriceImpact(bestSplits)

	return &entities.Quote{
		TokenIn:     tokenIn,
		TokenOut:    tokenOut,
		AmountIn:    amountIn,
		AmountOut:   bestSplitOutput,
		BestRoute:   bestRoute,
		SplitRoutes: bestSplits,
		PriceImpact: priceImpact,
		GasEstimate: bestGas + singleGas, // Extra gas for split
		Sources:     sources,
	}
}

// applySlippageProtection calculates minimum output amount based on slippage
func (s *RouterService) applySlippageProtection(quote *entities.Quote, slippageBps uint64) {
	if quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		return
	}

	// minAmountOut = amountOut * (10000 - slippageBps) / 10000
	multiplier := big.NewInt(10000 - int64(slippageBps))
	minAmount := new(big.Int).Mul(quote.AmountOut, multiplier)
	minAmount.Div(minAmount, big.NewInt(10000))

	quote.MinAmountOut = minAmount
	quote.SlippageBps = slippageBps
}

// filterValidPrices filters and sorts prices by output amount
func filterValidPrices(prices []PriceResult) []PriceResult {
	var valid []PriceResult
	for _, p := range prices {
		if p.Error == nil && p.AmountOut != nil && p.AmountOut.Sign() > 0 && p.Pair != nil {
			valid = append(valid, p)
		}
	}

	sort.Slice(valid, func(i, j int) bool {
		return valid[i].AmountOut.Cmp(valid[j].AmountOut) > 0
	})

	return valid
}

// calculateSplitPriceImpact calculates weighted average price impact for split routes
func calculateSplitPriceImpact(splits []entities.SplitRoute) *big.Int {
	if len(splits) == 0 {
		return big.NewInt(0)
	}

	totalWeight := big.NewInt(0)
	weightedImpact := big.NewInt(0)

	for _, split := range splits {
		if split.Route != nil {
			impact := split.Route.CalculatePriceImpact()
			weight := big.NewInt(int64(split.Percentage))
			totalWeight.Add(totalWeight, weight)
			weightedImpact.Add(weightedImpact, new(big.Int).Mul(impact, weight))
		}
	}

	if totalWeight.Sign() == 0 {
		return big.NewInt(0)
	}

	return new(big.Int).Div(weightedImpact, totalWeight)
}
