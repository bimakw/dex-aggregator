package services

import (
	"context"
	"fmt"
	"math/big"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
)

// RouterService handles route finding and quote generation
type RouterService struct {
	priceService *PriceService
}

// NewRouterService creates a new router service
func NewRouterService(priceService *PriceService) *RouterService {
	return &RouterService{
		priceService: priceService,
	}
}

// GetQuote finds the best route and returns a quote
func (s *RouterService) GetQuote(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int) (*entities.Quote, error) {
	// Get prices from all DEXes
	prices, err := s.priceService.GetPrices(ctx, tokenIn, tokenOut, amountIn)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	// Find best direct route
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

	// Build route
	route := s.buildRoute(tokenIn, tokenOut, amountIn, bestResult)

	// Calculate price impact
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

	// Base gas + gas per hop
	baseGas := uint64(21000)
	gasPerHop := uint64(100000) // Approximate gas for a Uniswap V2 swap

	return baseGas + uint64(len(route.Hops))*gasPerHop
}

// GetMultiHopQuote finds the best route including multi-hop paths (Phase 3)
func (s *RouterService) GetMultiHopQuote(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int, intermediateTokens []entities.Token) (*entities.Quote, error) {
	// First try direct route
	directQuote, directErr := s.GetQuote(ctx, tokenIn, tokenOut, amountIn)

	// Try multi-hop routes through intermediate tokens
	var bestQuote *entities.Quote
	if directErr == nil {
		bestQuote = directQuote
	}

	for _, intermediate := range intermediateTokens {
		if intermediate.Address == tokenIn.Address || intermediate.Address == tokenOut.Address {
			continue
		}

		// Get first hop: tokenIn -> intermediate
		hop1Prices, err := s.priceService.GetPrices(ctx, tokenIn, intermediate, amountIn)
		if err != nil {
			continue
		}

		for _, hop1 := range hop1Prices {
			if hop1.Error != nil || hop1.AmountOut == nil || hop1.AmountOut.Sign() <= 0 {
				continue
			}

			// Get second hop: intermediate -> tokenOut
			hop2Prices, err := s.priceService.GetPrices(ctx, intermediate, tokenOut, hop1.AmountOut)
			if err != nil {
				continue
			}

			for _, hop2 := range hop2Prices {
				if hop2.Error != nil || hop2.AmountOut == nil || hop2.AmountOut.Sign() <= 0 {
					continue
				}

				// Check if this multi-hop is better
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
