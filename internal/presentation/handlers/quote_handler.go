package handlers

import (
	"encoding/json"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	"github.com/bimakw/dex-aggregator/internal/domain/services"
)

type QuoteHandler struct {
	routerService *services.RouterService
	tokenRegistry map[common.Address]entities.Token
}

func NewQuoteHandler(routerService *services.RouterService) *QuoteHandler {
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

type QuoteRequest struct {
	TokenIn  string `json:"tokenIn"`
	TokenOut string `json:"tokenOut"`
	AmountIn string `json:"amountIn"`
}

type QuoteResponse struct {
	TokenIn      string            `json:"tokenIn"`
	TokenOut     string            `json:"tokenOut"`
	AmountIn     string            `json:"amountIn"`
	AmountOut    string            `json:"amountOut"`
	MinAmountOut string            `json:"minAmountOut,omitempty"`
	SlippageBps  uint64            `json:"slippageBps,omitempty"`
	Route        []RouteHop        `json:"route"`
	SplitRoutes  []SplitRouteResp  `json:"splitRoutes,omitempty"`
	PriceImpact  string            `json:"priceImpact"`
	PriceWarning string            `json:"priceWarning,omitempty"`
	GasEstimate  uint64            `json:"gasEstimate"`
	Sources      map[string]string `json:"sources"`
}

type SplitRouteResp struct {
	DEX        string `json:"dex"`
	Percentage uint64 `json:"percentage"`
	AmountIn   string `json:"amountIn"`
	AmountOut  string `json:"amountOut"`
}

type RouteHop struct {
	DEX      string `json:"dex"`
	Pair     string `json:"pair"`
	TokenIn  string `json:"tokenIn"`
	TokenOut string `json:"tokenOut"`
	Fee      uint64 `json:"fee"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (h *QuoteHandler) GetQuote(w http.ResponseWriter, r *http.Request) {
	tokenInAddr := r.URL.Query().Get("tokenIn")
	tokenOutAddr := r.URL.Query().Get("tokenOut")
	amountInStr := r.URL.Query().Get("amountIn")
	slippageStr := r.URL.Query().Get("slippage")

	if tokenInAddr == "" || tokenOutAddr == "" || amountInStr == "" {
		h.writeError(w, http.StatusBadRequest, "missing_params", "tokenIn, tokenOut, and amountIn are required")
		return
	}

	if !common.IsHexAddress(tokenInAddr) {
		h.writeError(w, http.StatusBadRequest, "invalid_token_in", "tokenIn is not a valid address")
		return
	}
	if !common.IsHexAddress(tokenOutAddr) {
		h.writeError(w, http.StatusBadRequest, "invalid_token_out", "tokenOut is not a valid address")
		return
	}

	amountIn, ok := new(big.Int).SetString(amountInStr, 10)
	if !ok || amountIn.Sign() <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_amount", "amountIn must be a positive integer")
		return
	}

	// Parse slippage (optional, in basis points, default 50 = 0.5%)
	var slippageBps uint64
	if slippageStr != "" {
		slippage, ok := new(big.Int).SetString(slippageStr, 10)
		if !ok || slippage.Sign() < 0 || slippage.Cmp(big.NewInt(10000)) > 0 {
			h.writeError(w, http.StatusBadRequest, "invalid_slippage", "slippage must be 0-10000 basis points")
			return
		}
		slippageBps = slippage.Uint64()
	}

	tokenIn, ok := h.tokenRegistry[common.HexToAddress(tokenInAddr)]
	if !ok {
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

	quote, err := h.routerService.GetSmartQuote(r.Context(), tokenIn, tokenOut, amountIn, slippageBps)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "no_route", err.Error())
		return
	}

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

	minAmountOut := ""
	if quote.MinAmountOut != nil {
		minAmountOut = quote.MinAmountOut.String()
	}

	var splitRoutes []SplitRouteResp
	for _, sr := range quote.SplitRoutes {
		dexType := ""
		if sr.Route != nil && len(sr.Route.Hops) > 0 {
			dexType = string(sr.Route.Hops[0].Pair.DEX)
		}
		splitRoutes = append(splitRoutes, SplitRouteResp{
			DEX:        dexType,
			Percentage: sr.Percentage,
			AmountIn:   sr.AmountIn.String(),
			AmountOut:  sr.AmountOut.String(),
		})
	}

	return QuoteResponse{
		TokenIn:      quote.TokenIn.Address.Hex(),
		TokenOut:     quote.TokenOut.Address.Hex(),
		AmountIn:     quote.AmountIn.String(),
		AmountOut:    quote.AmountOut.String(),
		MinAmountOut: minAmountOut,
		SlippageBps:  quote.SlippageBps,
		Route:        routeHops,
		SplitRoutes:  splitRoutes,
		PriceImpact:  priceImpactBps,
		PriceWarning: quote.PriceWarning,
		GasEstimate:  quote.GasEstimate,
		Sources:      sources,
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
