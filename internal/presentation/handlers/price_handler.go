package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
	"github.com/bimakw/dex-aggregator/internal/domain/services"
)

type PriceHandler struct {
	priceService  *services.PriceService
	tokenRegistry map[common.Address]entities.Token
}

func NewPriceHandler(priceService *services.PriceService) *PriceHandler {
	registry := map[common.Address]entities.Token{
		entities.WETH.Address: entities.WETH,
		entities.USDC.Address: entities.USDC,
		entities.USDT.Address: entities.USDT,
		entities.DAI.Address:  entities.DAI,
	}

	return &PriceHandler{
		priceService:  priceService,
		tokenRegistry: registry,
	}
}

type PriceResponse struct {
	Token     string            `json:"token"`
	Symbol    string            `json:"symbol"`
	PriceUSD  string            `json:"priceUSD"`
	Sources   map[string]string `json:"sources,omitempty"`
	UpdatedAt string            `json:"updatedAt"`
}

// GetPrice handles GET /api/v1/price/{tokenAddress}
func (h *PriceHandler) GetPrice(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		h.writeError(w, http.StatusBadRequest, "missing_token", "token address is required")
		return
	}
	tokenAddr := parts[len(parts)-1]

	if !common.IsHexAddress(tokenAddr) {
		h.writeError(w, http.StatusBadRequest, "invalid_token", "invalid token address")
		return
	}

	token, ok := h.tokenRegistry[common.HexToAddress(tokenAddr)]
	if !ok {
		token = entities.Token{
			Address:  common.HexToAddress(tokenAddr),
			Symbol:   "UNKNOWN",
			Decimals: 18,
		}
	}

	price, err := h.priceService.GetTokenPrice(r.Context(), token)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "price_not_found", err.Error())
		return
	}

	// Format price (18 decimals -> human readable)
	priceStr := formatPrice(price)

	response := PriceResponse{
		Token:     token.Address.Hex(),
		Symbol:    token.Symbol,
		PriceUSD:  priceStr,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	h.writeJSON(w, http.StatusOK, response)
}

// formatPrice formats a price with 18 decimals to a human-readable string
func formatPrice(price interface{ String() string }) string {
	priceStr := price.String()
	if len(priceStr) <= 18 {
		return "0." + strings.Repeat("0", 18-len(priceStr)) + priceStr
	}
	decimalPos := len(priceStr) - 18
	return priceStr[:decimalPos] + "." + priceStr[decimalPos:decimalPos+2]
}

func (h *PriceHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *PriceHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
	})
}
