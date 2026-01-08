package entities

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestGetSpotPrice(t *testing.T) {
	tests := []struct {
		name     string
		reserve0 *big.Int
		reserve1 *big.Int
		want     string
	}{
		{
			name:     "equal reserves",
			reserve0: big.NewInt(1000000),
			reserve1: big.NewInt(1000000),
			want:     "1000000000000000000", // 1e18
		},
		{
			name:     "2x price ratio",
			reserve0: big.NewInt(1000000),
			reserve1: big.NewInt(2000000),
			want:     "2000000000000000000", // 2e18
		},
		{
			name:     "0.5x price ratio",
			reserve0: big.NewInt(2000000),
			reserve1: big.NewInt(1000000),
			want:     "500000000000000000", // 0.5e18
		},
		{
			name:     "zero reserve0",
			reserve0: big.NewInt(0),
			reserve1: big.NewInt(1000000),
			want:     "0",
		},
		{
			name:     "nil reserves",
			reserve0: nil,
			reserve1: nil,
			want:     "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pair{
				Reserve0: tt.reserve0,
				Reserve1: tt.reserve1,
			}
			got := p.GetSpotPrice()
			if got.String() != tt.want {
				t.Errorf("GetSpotPrice() = %v, want %v", got.String(), tt.want)
			}
		})
	}
}

func TestGetAmountOut(t *testing.T) {
	token0 := common.HexToAddress("0x0000000000000000000000000000000000000001")
	token1 := common.HexToAddress("0x0000000000000000000000000000000000000002")

	tests := []struct {
		name     string
		reserve0 *big.Int
		reserve1 *big.Int
		fee      uint64
		amountIn *big.Int
		tokenIn  common.Address
		wantGT   string // result should be greater than this
		wantLT   string // result should be less than this
	}{
		{
			name:     "standard swap token0 to token1",
			reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)), // 10000 tokens
			reserve1: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			fee:      30,               // 0.3%
			amountIn: big.NewInt(1e18), // 1 token
			tokenIn:  token0,
			wantGT:   "990000000000000000",  // > 0.99 (fee impact)
			wantLT:   "1000000000000000000", // < 1 (price impact)
		},
		{
			name:     "swap token1 to token0",
			reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			reserve1: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			fee:      30,
			amountIn: big.NewInt(1e18),
			tokenIn:  token1,
			wantGT:   "990000000000000000",
			wantLT:   "1000000000000000000",
		},
		{
			name:     "large swap with price impact",
			reserve0: new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)), // 1000 tokens
			reserve1: new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)),
			fee:      30,
			amountIn: new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)), // 100 tokens (10%)
			tokenIn:  token0,
			wantGT:   "80000000000000000000",  // > 80 tokens
			wantLT:   "100000000000000000000", // < 100 tokens (significant impact)
		},
		{
			name:     "zero amount in",
			reserve0: big.NewInt(1e18),
			reserve1: big.NewInt(1e18),
			fee:      30,
			amountIn: big.NewInt(0),
			tokenIn:  token0,
			wantGT:   "-1", // >= 0
			wantLT:   "1",  // < 1
		},
		{
			name:     "different fee tier (0.05%)",
			reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			reserve1: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			fee:      5, // 0.05%
			amountIn: big.NewInt(1e18),
			tokenIn:  token0,
			wantGT:   "995000000000000000", // > 0.995 (lower fee = more output)
			wantLT:   "1000000000000000000",
		},
		{
			name:     "high fee tier (1%)",
			reserve0: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			reserve1: new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
			fee:      100, // 1%
			amountIn: big.NewInt(1e18),
			tokenIn:  token0,
			wantGT:   "980000000000000000", // > 0.98 (higher fee = less output)
			wantLT:   "1000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Pair{
				Token0:   Token{Address: token0},
				Token1:   Token{Address: token1},
				Reserve0: tt.reserve0,
				Reserve1: tt.reserve1,
				Fee:      tt.fee,
			}

			got := p.GetAmountOut(tt.amountIn, tt.tokenIn)

			wantGT, _ := new(big.Int).SetString(tt.wantGT, 10)
			wantLT, _ := new(big.Int).SetString(tt.wantLT, 10)

			if got.Cmp(wantGT) <= 0 {
				t.Errorf("GetAmountOut() = %v, want > %v", got.String(), tt.wantGT)
			}
			if got.Cmp(wantLT) >= 0 {
				t.Errorf("GetAmountOut() = %v, want < %v", got.String(), tt.wantLT)
			}
		})
	}
}

func TestGetAmountOutWithNilReserves(t *testing.T) {
	p := &Pair{
		Token0:   Token{Address: common.HexToAddress("0x1")},
		Token1:   Token{Address: common.HexToAddress("0x2")},
		Reserve0: nil,
		Reserve1: nil,
		Fee:      30,
	}

	got := p.GetAmountOut(big.NewInt(1e18), p.Token0.Address)
	if got.Sign() != 0 {
		t.Errorf("GetAmountOut() with nil reserves = %v, want 0", got.String())
	}
}

func TestGetAmountOutWithZeroReserves(t *testing.T) {
	p := &Pair{
		Token0:   Token{Address: common.HexToAddress("0x1")},
		Token1:   Token{Address: common.HexToAddress("0x2")},
		Reserve0: big.NewInt(0),
		Reserve1: big.NewInt(1000),
		Fee:      30,
	}

	got := p.GetAmountOut(big.NewInt(1e18), p.Token0.Address)
	if got.Sign() != 0 {
		t.Errorf("GetAmountOut() with zero reserveIn = %v, want 0", got.String())
	}
}
