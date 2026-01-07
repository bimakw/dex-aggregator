package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bimakw/dex-aggregator/internal/domain/services"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/cache"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/dex"
	"github.com/bimakw/dex-aggregator/internal/infrastructure/ethereum"
	"github.com/bimakw/dex-aggregator/internal/presentation/handlers"
)

const (
	version = "0.2.0"
)

func main() {
	// Get configuration from environment
	rpcURL := getEnv("ETH_RPC_URL", "https://eth.llamarpc.com")
	redisAddr := getEnv("REDIS_ADDR", "")
	port := getEnv("PORT", "8080")

	// Initialize Ethereum client
	ethClient, err := ethereum.NewClient(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum: %v", err)
	}
	defer ethClient.Close()
	log.Printf("Connected to Ethereum (chain ID: %s)", ethClient.ChainID().String())

	// Initialize cache
	var cacheClient cache.Cache
	if redisAddr != "" {
		redisCache, err := cache.NewRedisCache(redisAddr, "", 0)
		if err != nil {
			log.Printf("Warning: Failed to connect to Redis: %v. Using in-memory cache.", err)
			cacheClient = cache.NewInMemoryCache()
		} else {
			cacheClient = redisCache
			log.Printf("Connected to Redis at %s", redisAddr)
		}
	} else {
		cacheClient = cache.NewInMemoryCache()
		log.Println("Using in-memory cache")
	}

	// Initialize DEX clients
	uniswapV2 := dex.NewUniswapV2Client(ethClient)
	uniswapV3 := dex.NewUniswapV3Client(ethClient)
	sushiswap := dex.NewSushiswapClient(ethClient)
	dexClients := []dex.DEXClient{uniswapV2, uniswapV3, sushiswap}

	// Initialize services
	priceService := services.NewPriceService(dexClients, cacheClient)
	routerService := services.NewRouterService(priceService)

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler(version)
	quoteHandler := handlers.NewQuoteHandler(routerService)
	priceHandler := handlers.NewPriceHandler(priceService)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	// Routes
	r.Get("/health", healthHandler.Health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/quote", quoteHandler.GetQuote)
		r.Get("/price/{tokenAddress}", priceHandler.GetPrice)
	})

	// Start server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting DEX Aggregator API v%s on port %s", version, port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
