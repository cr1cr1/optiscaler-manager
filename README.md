# optiscaler-manager

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games — download the current bundle, install it into a
game directory with full backup + rollback safety, and uninstall cleanly.

GUI: [go-shirei](https://github.com/hasenj/go-shirei) (`go.hasen.dev/shirei`,
pinned). v0.1 targets **Linux + Steam (Proton)**.

Status: early development. See `docs/` for scope, architecture, safety model,
and the milestone plan.

## Usage (planned)

```
optiscaler-manager            # launch the GUI
optiscaler-manager scan       # list discovered games
optiscaler-manager install <path>    # headless install into a game dir
optiscaler-manager uninstall <path>  # headless, SHA-verified removal
```

## Development

```
go test ./...     # full verification (the only sanctioned test entrypoint)
go vet ./...
```

Conventions: TDD first, zerolog in production code, `t.Log` in tests,
OKF-formatted docs under `docs/`, commit per completed task. See `AGENTS.md`.

---

## License

[MIT License](LICENSE)
