package services

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/dex"
)

// MockDEXClient is a mock implementation of DEXClient for testing
type MockDEXClient struct {
	dexType   entities.DEXType
	pairs     map[string]*entities.Pair
	amountOut *big.Int
	err       error
}

func NewMockDEXClient(dexType entities.DEXType) *MockDEXClient {
	return &MockDEXClient{
		dexType: dexType,
		pairs:   make(map[string]*entities.Pair),
	}
}

func (m *MockDEXClient) SetPair(tokenA, tokenB common.Address, pair *entities.Pair) {
	key := pairKey(tokenA, tokenB)
	m.pairs[key] = pair
}

func (m *MockDEXClient) SetAmountOut(amount *big.Int) {
	m.amountOut = amount
}

func (m *MockDEXClient) SetError(err error) {
	m.err = err
}

func pairKey(tokenA, tokenB common.Address) string {
	if tokenA.Hex() < tokenB.Hex() {
		return tokenA.Hex() + "-" + tokenB.Hex()
	}
	return tokenB.Hex() + "-" + tokenA.Hex()
}

func (m *MockDEXClient) GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	if m.err != nil {
		return common.Address{}, m.err
	}
	key := pairKey(tokenA, tokenB)
	if pair, ok := m.pairs[key]; ok {
		return pair.Address, nil
	}
	return common.Address{}, nil
}

func (m *MockDEXClient) GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := pairKey(tokenA.Address, tokenB.Address)
	if pair, ok := m.pairs[key]; ok {
		return pair, nil
	}
	return nil, nil
}

func (m *MockDEXClient) GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.amountOut != nil {
		return m.amountOut, nil
	}
	// Use pair's GetAmountOut if available
	key := pairKey(tokenIn.Address, tokenOut.Address)
	if pair, ok := m.pairs[key]; ok {
		return pair.GetAmountOut(amountIn, tokenIn.Address), nil
	}
	return big.NewInt(0), nil
}

func (m *MockDEXClient) DEXType() entities.DEXType {
	return m.dexType
}

// MockCache is a mock implementation of Cache for testing
type MockCache struct{}

func (m *MockCache) GetPair(ctx context.Context, key string) (*entities.Pair, error) {
	return nil, nil // Always miss
}

func (m *MockCache) SetPair(ctx context.Context, key string, pair *entities.Pair, ttl time.Duration) error {
	return nil
}

func (m *MockCache) GetPrice(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *MockCache) SetPrice(ctx context.Context, key string, price string, ttl time.Duration) error {
	return nil
}

func (m *MockCache) Delete(ctx context.Context, key string) error {
	return nil
}

func TestRouterServiceGetQuote(t *testing.T) {
	token0 := entities.Token{
		Address:  common.HexToAddress("0x0000000000000000000000000000000000000001"),
		Symbol:   "TOKEN0",
		Decimals: 18,
	}
	token1 := entities.Token{
		Address:  common.HexToAddress("0x0000000000000000000000000000000000000002"),
		Symbol:   "TOKEN1",
		Decimals: 18,
	}

	// Create mock DEX clients
	mockV2 := NewMockDEXClient(entities.DEXUniswapV2)
	mockV3 := NewMockDEXClient(entities.DEXUniswapV3)
	mockSushi := NewMockDEXClient(entities.DEXSushiswap)

	// Set up pairs with different reserves (different prices)
	pairV2 := &entities.Pair{
		Address:  common.HexToAddress("0x1111"),
		Token0:   token0,
		Token1:   token1,
		Reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		Reserve1: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		DEX:      entities.DEXUniswapV2,
		Fee:      30, // 0.3%
	}

	pairV3 := &entities.Pair{
		Address:  common.HexToAddress("0x2222"),
		Token0:   token0,
		Token1:   token1,
		Reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		Reserve1: new(big.Int).Mul(big.NewInt(10200), big.NewInt(1e18)), // Slightly better rate
		DEX:      entities.DEXUniswapV3,
		Fee:      5, // 0.05%
	}

	pairSushi := &entities.Pair{
		Address:  common.HexToAddress("0x3333"),
		Token0:   token0,
		Token1:   token1,
		Reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		Reserve1: new(big.Int).Mul(big.NewInt(9900), big.NewInt(1e18)), // Slightly worse rate
		DEX:      entities.DEXSushiswap,
		Fee:      30, // 0.3%
	}

	mockV2.SetPair(token0.Address, token1.Address, pairV2)
	mockV3.SetPair(token0.Address, token1.Address, pairV3)
	mockSushi.SetPair(token0.Address, token1.Address, pairSushi)

	// Create services
	dexClients := []dex.DEXClient{mockV2, mockV3, mockSushi}
	priceService := NewPriceService(dexClients, &MockCache{})
	routerService := NewRouterService(priceService)

	// Test GetQuote
	amountIn := big.NewInt(1e18) // 1 token
	quote, err := routerService.GetQuote(context.Background(), token0, token1, amountIn)

	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}

	if quote == nil {
		t.Fatal("Quote is nil")
	}

	// Verify we have sources
	if len(quote.Sources) == 0 {
		t.Error("No sources in quote")
	}

	// Verify best route was selected (V3 should be best due to better reserves)
	if quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		t.Error("AmountOut should be positive")
	}

	t.Logf("Quote: AmountIn=%s, AmountOut=%s, BestDEX=%s",
		quote.AmountIn.String(), quote.AmountOut.String(), quote.BestRoute.Hops[0].Pair.DEX)
	t.Logf("Sources: %v", quote.Sources)
}

func TestRouterServiceNoValidRoutes(t *testing.T) {
	token0 := entities.Token{
		Address:  common.HexToAddress("0x0000000000000000000000000000000000000001"),
		Symbol:   "TOKEN0",
		Decimals: 18,
	}
	token1 := entities.Token{
		Address:  common.HexToAddress("0x0000000000000000000000000000000000000002"),
		Symbol:   "TOKEN1",
		Decimals: 18,
	}

	// Create mock DEX client that returns error
	mock := NewMockDEXClient(entities.DEXUniswapV2)
	mock.SetError(context.DeadlineExceeded)

	dexClients := []dex.DEXClient{mock}
	priceService := NewPriceService(dexClients, &MockCache{})
	routerService := NewRouterService(priceService)

	_, err := routerService.GetQuote(context.Background(), token0, token1, big.NewInt(1e18))
	if err == nil {
		t.Error("Expected error for no valid routes, got nil")
	}
}

func TestEstimateGas(t *testing.T) {
	tests := []struct {
		name string
		hops int
		want uint64
	}{
		{"nil route", 0, 150000},
		{"single hop", 1, 121000}, // 21000 + 100000
		{"two hops", 2, 221000},   // 21000 + 200000
		{"three hops", 3, 321000}, // 21000 + 300000
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var route *entities.Route
			if tt.hops > 0 {
				route = &entities.Route{
					Hops: make([]entities.Hop, tt.hops),
				}
			}

			got := estimateGas(route)
			if got != tt.want {
				t.Errorf("estimateGas() = %v, want %v", got, tt.want)
			}
		})
	}
}
