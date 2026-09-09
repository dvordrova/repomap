# Cumulative Go repository

Two separate services use pkg/servicecfg. That package is supporting code for
the services; it is not a separately shipped library product.

## API service

```sh
go run ./cmd/api
```

## Background worker service

```sh
go run ./cmd/worker
```

cmd/app is the existing repomap development exercise. _examples/published is a
usage example with its own Go module. Both remain available for analysis.
