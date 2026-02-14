package dex

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
)

// DEXClient defines the interface for interacting with a DEX
type DEXClient interface {
	GetPairAddress(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error)

	GetPairByTokens(ctx context.Context, tokenA, tokenB entities.Token) (*entities.Pair, error)

	GetAmountOut(ctx context.Context, amountIn *big.Int, tokenIn, tokenOut entities.Token) (*big.Int, error)

	// DEXType returns the type of DEX
	DEXType() entities.DEXType
}
