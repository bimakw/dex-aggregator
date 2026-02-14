package entities

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

// TokenConfig represents token configuration from JSON
type TokenConfig struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals uint8  `json:"decimals"`
}

type TokensConfig struct {
	Tokens []TokenConfig `json:"tokens"`
}

type TokenRegistry struct {
	byAddress map[common.Address]Token
	bySymbol  map[string]Token
	all       []Token
}

func NewTokenRegistry() *TokenRegistry {
	return &TokenRegistry{
		byAddress: make(map[common.Address]Token),
		bySymbol:  make(map[string]Token),
		all:       make([]Token, 0),
	}
}

func (r *TokenRegistry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read token config: %w", err)
	}

	var config TokensConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse token config: %w", err)
	}

	for _, tc := range config.Tokens {
		token := Token{
			Address:  common.HexToAddress(tc.Address),
			Symbol:   tc.Symbol,
			Name:     tc.Name,
			Decimals: tc.Decimals,
		}
		r.Register(token)
	}

	return nil
}

func (r *TokenRegistry) Register(token Token) {
	r.byAddress[token.Address] = token
	r.bySymbol[token.Symbol] = token
	r.all = append(r.all, token)
}

func (r *TokenRegistry) GetByAddress(addr common.Address) (Token, bool) {
	token, ok := r.byAddress[addr]
	return token, ok
}

func (r *TokenRegistry) GetBySymbol(symbol string) (Token, bool) {
	token, ok := r.bySymbol[symbol]
	return token, ok
}

func (r *TokenRegistry) GetAll() []Token {
	return r.all
}

func (r *TokenRegistry) Count() int {
	return len(r.all)
}

func DefaultRegistry() *TokenRegistry {
	r := NewTokenRegistry()
	r.Register(WETH)
	r.Register(USDC)
	r.Register(USDT)
	r.Register(DAI)
	return r
}
