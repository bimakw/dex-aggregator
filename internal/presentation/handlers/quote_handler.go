package handlers

import (
	"encoding/json"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	"github.com/bimakw/dex-aggregator/internal/domain/services"
)

// QuoteHandler handles quote requests
type QuoteHandler struct {
	routerService *services.RouterService
	tokenRegistry map[common.Address]entities.Token
}

// NewQuoteHandler creates a new quote handler
func NewQuoteHandler(routerService *services.RouterService) *QuoteHandler {
	// Initialize token registry with common tokens
	registry := map[common.Address]entities.Token{
		entities.WETH.Address: entities.WETH,
		entities.USDC.Address: entities.USDC,
		entities.USDT.Address: entities.USDT,
		entities.DAI.Address:  entities.DAI,
	}

	return &QuoteHandler{
		routerService: routerService,
		tokenRegistry: registry,
	}
}

// QuoteRequest represents a quote request
type QuoteRequest struct {
	TokenIn  string `json:"tokenIn"`
	TokenOut string `json:"tokenOut"`
	AmountIn string `json:"amountIn"`
}

// QuoteResponse represents a quote response
type QuoteResponse struct {
	TokenIn     string              `json:"tokenIn"`
	TokenOut    string              `json:"tokenOut"`
	AmountIn    string              `json:"amountIn"`
	AmountOut   string              `json:"amountOut"`
	Route       []RouteHop          `json:"route"`
	PriceImpact string              `json:"priceImpact"`
	GasEstimate uint64              `json:"gasEstimate"`
	Sources     map[string]string   `json:"sources"`
}

// RouteHop represents a hop in the route
type RouteHop struct {
	DEX      string `json:"dex"`
	Pair     string `json:"pair"`
	TokenIn  string `json:"tokenIn"`
	TokenOut string `json:"tokenOut"`
	Fee      uint64 `json:"fee"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// GetQuote handles GET /api/v1/quote
func (h *QuoteHandler) GetQuote(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	tokenInAddr := r.URL.Query().Get("tokenIn")
	tokenOutAddr := r.URL.Query().Get("tokenOut")
	amountInStr := r.URL.Query().Get("amountIn")

	if tokenInAddr == "" || tokenOutAddr == "" || amountInStr == "" {
		h.writeError(w, http.StatusBadRequest, "missing_params", "tokenIn, tokenOut, and amountIn are required")
		return
	}

	// Validate addresses
	if !common.IsHexAddress(tokenInAddr) {
		h.writeError(w, http.StatusBadRequest, "invalid_token_in", "tokenIn is not a valid address")
		return
	}
	if !common.IsHexAddress(tokenOutAddr) {
		h.writeError(w, http.StatusBadRequest, "invalid_token_out", "tokenOut is not a valid address")
		return
	}

	// Parse amount
	amountIn, ok := new(big.Int).SetString(amountInStr, 10)
	if !ok || amountIn.Sign() <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_amount", "amountIn must be a positive integer")
		return
	}

	// Look up tokens
	tokenIn, ok := h.tokenRegistry[common.HexToAddress(tokenInAddr)]
	if !ok {
		// Create generic token if not in registry
		tokenIn = entities.Token{
			Address:  common.HexToAddress(tokenInAddr),
			Symbol:   "UNKNOWN",
			Decimals: 18,
		}
	}

	tokenOut, ok := h.tokenRegistry[common.HexToAddress(tokenOutAddr)]
	if !ok {
		tokenOut = entities.Token{
			Address:  common.HexToAddress(tokenOutAddr),
			Symbol:   "UNKNOWN",
			Decimals: 18,
		}
	}

	// Get quote
	quote, err := h.routerService.GetQuote(r.Context(), tokenIn, tokenOut, amountIn)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "no_route", err.Error())
		return
	}

	// Build response
	response := h.buildQuoteResponse(quote)
	h.writeJSON(w, http.StatusOK, response)
}

// buildQuoteResponse converts a Quote to a QuoteResponse
func (h *QuoteHandler) buildQuoteResponse(quote *entities.Quote) QuoteResponse {
	var routeHops []RouteHop
	if quote.BestRoute != nil {
		for _, hop := range quote.BestRoute.Hops {
			routeHops = append(routeHops, RouteHop{
				DEX:      string(hop.Pair.DEX),
				Pair:     hop.Pair.Address.Hex(),
				TokenIn:  hop.TokenIn.Hex(),
				TokenOut: hop.TokenOut.Hex(),
				Fee:      hop.Pair.Fee,
			})
		}
	}

	sources := make(map[string]string)
	for dex, amount := range quote.Sources {
		sources[string(dex)] = amount
	}

	priceImpactBps := "0"
	if quote.PriceImpact != nil {
		priceImpactBps = quote.PriceImpact.String()
	}

	return QuoteResponse{
		TokenIn:     quote.TokenIn.Address.Hex(),
		TokenOut:    quote.TokenOut.Address.Hex(),
		AmountIn:    quote.AmountIn.String(),
		AmountOut:   quote.AmountOut.String(),
		Route:       routeHops,
		PriceImpact: priceImpactBps,
		GasEstimate: quote.GasEstimate,
		Sources:     sources,
	}
}

func (h *QuoteHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *QuoteHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
	})
}
