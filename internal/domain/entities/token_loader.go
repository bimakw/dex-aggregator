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

// TokensConfig represents the tokens.json structure
type TokensConfig struct {
	Tokens []TokenConfig `json:"tokens"`
}

// TokenRegistry holds loaded tokens indexed by address and symbol
type TokenRegistry struct {
	byAddress map[common.Address]Token
	bySymbol  map[string]Token
	all       []Token
}

// NewTokenRegistry creates a new token registry
func NewTokenRegistry() *TokenRegistry {
	return &TokenRegistry{
		byAddress: make(map[common.Address]Token),
		bySymbol:  make(map[string]Token),
		all:       make([]Token, 0),
	}
}

// LoadFromFile loads tokens from a JSON config file
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

// Register adds a token to the registry
func (r *TokenRegistry) Register(token Token) {
	r.byAddress[token.Address] = token
	r.bySymbol[token.Symbol] = token
	r.all = append(r.all, token)
}

// GetByAddress returns a token by its address
func (r *TokenRegistry) GetByAddress(addr common.Address) (Token, bool) {
	token, ok := r.byAddress[addr]
	return token, ok
}

// GetBySymbol returns a token by its symbol
func (r *TokenRegistry) GetBySymbol(symbol string) (Token, bool) {
	token, ok := r.bySymbol[symbol]
	return token, ok
}

// GetAll returns all registered tokens
func (r *TokenRegistry) GetAll() []Token {
	return r.all
}

// Count returns the number of registered tokens
func (r *TokenRegistry) Count() int {
	return len(r.all)
}

// DefaultRegistry returns a registry with hardcoded default tokens
// Use this as fallback if config file is not available
func DefaultRegistry() *TokenRegistry {
	r := NewTokenRegistry()
	r.Register(WETH)
	r.Register(USDC)
	r.Register(USDT)
	r.Register(DAI)
	return r
}
