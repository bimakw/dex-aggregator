# DEX Price Aggregator

Aggregates token prices from Uniswap V2 and Sushiswap, finds optimal swap routes, and calculates price impact. Go + Redis (optional).

## Running

```bash
make dev
```

Or with Docker:

```bash
docker-compose up -d
```

## Endpoints

- `GET /api/v1/quote?tokenIn=&tokenOut=&amountIn=` — best swap route
- `GET /api/v1/price/{tokenAddress}` — USD price
- `GET /health`

Set `ETH_RPC_URL` for a custom RPC endpoint, `REDIS_ADDR` for persistent caching.

## Testing

```bash
go test ./...
```

## License

MIT
