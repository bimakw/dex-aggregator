package services

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/cache"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/dex"
)

type PriceService struct {
	dexClients []dex.DEXClient
	cache      cache.Cache
	cacheTTL   time.Duration
}

func NewPriceService(dexClients []dex.DEXClient, c cache.Cache) *PriceService {
	return &PriceService{
		dexClients: dexClients,
		cache:      c,
		cacheTTL:   10 * time.Second, // Short TTL for price data
	}
}

// PriceResult contains price data from a DEX
type PriceResult struct {
	DEX       entities.DEXType
	AmountOut *big.Int
	Pair      *entities.Pair
	Error     error
}

func (s *PriceService) GetPrices(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int) ([]PriceResult, error) {
	results := make([]PriceResult, len(s.dexClients))
	var wg sync.WaitGroup

	for i, client := range s.dexClients {
		wg.Add(1)
		go func(idx int, c dex.DEXClient) {
			defer wg.Done()

			cacheKey := cache.PairCacheKey(c.DEXType(), tokenIn.Address.Hex(), tokenOut.Address.Hex())

			if s.cache != nil {
				if cachedPair, err := s.cache.GetPair(ctx, cacheKey); err == nil && cachedPair != nil {
					amountOut := cachedPair.GetAmountOut(amountIn, tokenIn.Address)
					results[idx] = PriceResult{
						DEX:       c.DEXType(),
						AmountOut: amountOut,
						Pair:      cachedPair,
					}
					return
				}
			}

			// Fetch from DEX
			pair, err := c.GetPairByTokens(ctx, tokenIn, tokenOut)
			if err != nil {
				results[idx] = PriceResult{
					DEX:   c.DEXType(),
					Error: err,
				}
				return
			}

			if s.cache != nil {
				_ = s.cache.SetPair(ctx, cacheKey, pair, s.cacheTTL)
			}

			amountOut := pair.GetAmountOut(amountIn, tokenIn.Address)
			results[idx] = PriceResult{
				DEX:       c.DEXType(),
				AmountOut: amountOut,
				Pair:      pair,
			}
		}(i, client)
	}

	wg.Wait()
	return results, nil
}

func (s *PriceService) GetBestPrice(ctx context.Context, tokenIn, tokenOut entities.Token, amountIn *big.Int) (*PriceResult, error) {
	prices, err := s.GetPrices(ctx, tokenIn, tokenOut, amountIn)
	if err != nil {
		return nil, err
	}

	var best *PriceResult
	for i := range prices {
		if prices[i].Error != nil {
			continue
		}
		if prices[i].AmountOut == nil {
			continue
		}
		if best == nil || prices[i].AmountOut.Cmp(best.AmountOut) > 0 {
			best = &prices[i]
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no valid prices found")
	}

	return best, nil
}

// GetTokenPrice returns the price of a token in USD (using stablecoins as reference)
func (s *PriceService) GetTokenPrice(ctx context.Context, token entities.Token) (*big.Int, error) {
	if token.Address == entities.USDC.Address {
		// USDC price is $1 (represented as 10^18 for 18 decimal precision)
		return new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), nil
	}

	// Try direct pair with USDC
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(token.Decimals)), nil)
	best, err := s.GetBestPrice(ctx, token, entities.USDC, oneToken)
	if err == nil && best.AmountOut != nil && best.AmountOut.Sign() > 0 {
		price := new(big.Int).Mul(best.AmountOut, new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil))
		return price, nil
	}

	// Try via WETH
	if token.Address != entities.WETH.Address {
		wethResult, err := s.GetBestPrice(ctx, token, entities.WETH, oneToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get price: %w", err)
		}

		wethPrice, err := s.GetTokenPrice(ctx, entities.WETH)
		if err != nil {
			return nil, fmt.Errorf("failed to get WETH price: %w", err)
		}

		// price = (token/WETH) * (WETH/USD)
		price := new(big.Int).Mul(wethResult.AmountOut, wethPrice)
		price.Div(price, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		return price, nil
	}

	return nil, fmt.Errorf("unable to determine price for token %s", token.Symbol)
}
