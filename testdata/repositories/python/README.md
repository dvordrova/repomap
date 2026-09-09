# Cumulative Python repository

The acme distribution contains two independently deployed services. Sharing a
Python distribution does not merge their processes or deployment lifecycles.

## API service

```sh
PYTHONPATH=src python -m acme.api
```

## Background worker service

```sh
PYTHONPATH=src python -m acme.worker
```

`acme.demo` contains an incidental demonstration guard, not a separate product.
`acme.excluded.check` is a local diagnostic utility excluded from packaging.
The existing fixture_app console command is a development fixture for repomap.
