# shippo

A daemon that automatically exposes localhost LISTEN ports via `tailscale serve`, powered by eBPF for instant, event-driven detection.

## How it works

```txt
┌─────────────────────────────────────────────┐
│  eBPF tracepoint: sock/inet_sock_set_state  │
│  → Detects LISTEN start/stop via ring buffer│
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│  shippo daemon                              │
│  - LISTEN start → tailscale serve           │
│  - LISTEN stop  → tailscale serve off       │
└─────────────────────────────────────────────┘
```

- **No polling.** Pure event-driven using an eBPF tracepoint.
- **Only allowed ports** are exposed. No accidental DB or admin service leaks.
- Ports are served at `https://<machine>.<tailnet>/p<PORT>`.

## Requirements

- Linux kernel 5.8+
- Go 1.21+
- clang/llvm and libbpf headers (for eBPF compilation)
- Tailscale installed and authenticated

## Build

```bash
sudo apt install clang llvm libbpf-dev  # if not already installed
make build
```

`make build` runs `gobee` to translate the eBPF Go source before compiling the BPF object.

## Usage

```bash
# Run directly (requires root or capabilities)
sudo ./shippo

# Install and enable as systemd user service
make enable

# Check status
make status

# View logs
make logs

# Disable
make disable
```

## Configuration

Allowed ports can be configured in three ways (in priority order):

### 1. Environment variable

```bash
SHIPPO_PORTS=3000,5173,8000 ./shippo
```

### 2. Config file

`~/.config/shippo/config.json`:

```json
{
  "ports": [3000, 5173, 8000]
}
```

### 3. Default

If neither is set, defaults to `3000, 5173, 8000`.

## Capabilities

eBPF tracepoints require elevated privileges. The systemd service uses `AmbientCapabilities`:

- `CAP_BPF` – load BPF programs
- `CAP_PERFMON` – use ring buffer
- `CAP_NET_ADMIN` – network BPF operations

## License

MIT
