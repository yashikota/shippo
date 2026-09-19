# shippo

A daemon that automatically exposes localhost LISTEN ports via `tailscale serve`, powered by eBPF for instant, event-driven detection.

## Requirements

- Linux kernel 5.8+
- Tailscale installed and authenticated

## Install

Download the archive for your CPU from [GitHub Releases](https://github.com/yashikota/shippo/releases) (`linux_amd64` or `linux_arm64`).

```bash
tar xzf shippo_*_linux_amd64.tar.gz
mkdir -p ~/bin ~/.config/systemd/user
mv shippo ~/bin/
cp shippo.service ~/.config/systemd/user/
sudo setcap cap_bpf,cap_net_admin,cap_sys_ptrace,cap_perfmon=ep ~/bin/shippo
```

## Setup

After placing the binary, run these in order:

```bash
shippo init
sudo tailscale set --operator="$USER"
systemctl --user daemon-reload
systemctl --user enable --now shippo.service
```

`shippo init` chooses the ports to expose. No root required. Enter `*` to allow all localhost ports. Confirm with `y` to save.

Verify:

```bash
shippo status
tailscale serve status
```

Start a server on an allowed localhost port. It should appear at `https://<machine>.<tailnet>:<PORT>`.

## How it works

```txt
┌─────────────────────────────────────────────────┐
│  eBPF tracepoint: sock/inet_sock_set_state      │
│  → LISTEN start/stop events via ring buffer     │
└──────────────────┬──────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────┐
│  shippo daemon                                  │
│  - LISTEN start → tailscale serve --https=PORT  │
│  - LISTEN stop  → tailscale serve off           │
└─────────────────────────────────────────────────┘
```

- Event-driven via eBPF tracepoint, no polling
- Only allowlisted ports are exposed
- Served at `https://<machine>.<tailnet>:<PORT>`

## Usage

### Commands

```bash
shippo init             # choose allowed ports interactively
shippo status           # show listening ports and allowlist
shippo add 4000         # add to allowlist
shippo remove 4000      # remove from allowlist
shippo daemon           # run in foreground (systemd is usual)
```

### Service management

```bash
systemctl --user status shippo.service
journalctl --user -u shippo.service -f
systemctl --user disable --now shippo.service
```

## Configuration

Allowed ports are resolved in this priority order:

1. `SHIPPO_PORTS` environment variable (e.g. `SHIPPO_PORTS=3000,8000-8999 shippo daemon`)
2. Config file `~/.config/shippo/config.json`

```json
{
  "ports": ["3000", "5173", "8000-8999"]
}
```

## Capabilities

eBPF tracepoints require elevated privileges. Release installs and `task install` grant them with `setcap` on the binary:

- `CAP_BPF` – load BPF programs
- `CAP_PERFMON` – use ring buffer
- `CAP_NET_ADMIN` – network BPF operations
- `CAP_SYS_PTRACE` – attach tracepoints

Tailscale Serve also requires `sudo tailscale set --operator="$USER"`.

## Development

```bash
sudo apt install clang llvm libbpf-dev
git clone https://github.com/yashikota/shippo.git
cd shippo
aqua i
task enable
task check
task lint
task test
```

See [TEST.md](TEST.md) for integration test details.
