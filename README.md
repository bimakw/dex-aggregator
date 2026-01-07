# DEX Price Aggregator

API service that aggregates token prices from multiple decentralized exchanges and finds optimal swap routes.

## Features

- **Multi-DEX Price Fetching**: Query prices from Uniswap V2, Sushiswap
- **Best Route Finding**: Find the optimal swap route for best execution price
- **Price Impact Calculation**: Calculate slippage and price impact
- **Caching**: Redis or in-memory caching for improved performance
- **REST API**: Simple HTTP API for price quotes

## Supported DEXes

- Uniswap V2
- Sushiswap
- (Uniswap V3 - coming soon)

## Quick Start

### Prerequisites

- Go 1.23+
- Docker (optional)
- Redis (optional, for persistent caching)

### Run Locally

```bash
# Clone the repository
git clone https://github.com/bimakw/dex-aggregator.git
cd dex-aggregator

# Run with default public RPC
make dev

# Or with custom RPC
ETH_RPC_URL=https://your-rpc-url make dev
```

### Run with Docker

```bash
# Build and run with docker-compose
docker-compose up -d

# Or build manually
make docker-build
make docker-run
```

## API Endpoints

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

### Get Quote

```
GET /api/v1/quote?tokenIn=<address>&tokenOut=<address>&amountIn=<wei>
```

Example:
```bash
curl "http://localhost:8080/api/v1/quote?tokenIn=0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2&tokenOut=0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48&amountIn=1000000000000000000"
```

Response:
```json
{
  "tokenIn": "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
  "tokenOut": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
  "amountIn": "1000000000000000000",
  "amountOut": "3245678900",
  "route": [
    {
      "dex": "uniswap_v2",
      "pair": "0x...",
      "tokenIn": "WETH",
      "tokenOut": "USDC",
      "fee": 30
    }
  ],
  "priceImpact": "5",
  "gasEstimate": 121000,
  "sources": {
    "uniswap_v2": "3245678900",
    "sushiswap": "3240000000"
  }
}
```

### Get Token Price

```
GET /api/v1/price/{tokenAddress}
```

Response:
```json
{
  "token": "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
  "symbol": "WETH",
  "priceUSD": "3245.67",
  "updatedAt": "2026-01-07T12:00:00Z"
}
```

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `ETH_RPC_URL` | Ethereum RPC endpoint | `https://eth.llamarpc.com` |
| `REDIS_ADDR` | Redis address (optional) | (empty, uses in-memory) |
| `PORT` | HTTP server port | `8080` |

## Project Structure

```
dex-aggregator/
├── cmd/api/              # Application entrypoint
├── internal/
│   ├── domain/
│   │   ├── entities/     # Core domain models
│   │   └── services/     # Business logic
│   ├── infrastructure/
│   │   ├── cache/        # Redis/in-memory cache
│   │   ├── dex/          # DEX clients
│   │   └── ethereum/     # Ethereum RPC client
│   └── presentation/
│       └── handlers/     # HTTP handlers
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── README.md
```

## Roadmap

- [x] Phase 1: Core functionality (Uniswap V2, basic API)
- [ ] Phase 2: Multi-DEX (Uniswap V3, more sources)
- [ ] Phase 3: Advanced features (multi-hop routing, WebSocket feeds)

## License

MIT License - see [LICENSE](LICENSE) for details.
