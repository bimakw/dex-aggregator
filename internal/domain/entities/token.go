package entities

import "github.com/ethereum/go-ethereum/common"

type Token struct {
	Address  common.Address `json:"address"`
	Symbol   string         `json:"symbol"`
	Name     string         `json:"name"`
	Decimals uint8          `json:"decimals"`
}

// WETH is the canonical Wrapped Ether token on Ethereum mainnet
var WETH = Token{
	Address:  common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"),
	Symbol:   "WETH",
	Name:     "Wrapped Ether",
	Decimals: 18,
}

// USDC is USD Coin on Ethereum mainnet
var USDC = Token{
	Address:  common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"),
	Symbol:   "USDC",
	Name:     "USD Coin",
	Decimals: 6,
}

// USDT is Tether USD on Ethereum mainnet
var USDT = Token{
	Address:  common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7"),
	Symbol:   "USDT",
	Name:     "Tether USD",
	Decimals: 6,
}

// DAI is Dai Stablecoin on Ethereum mainnet
var DAI = Token{
	Address:  common.HexToAddress("0x6B175474E89094C44Da98b954EesfdfdAD3Ef9FB"),
	Symbol:   "DAI",
	Name:     "Dai Stablecoin",
	Decimals: 18,
}
