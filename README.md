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
- Ports are served at `https://<machine>.<tailnet>:<PORT>`.

## Requirements

- Linux kernel 5.8+
- Tailscale installed and authenticated

## Install

### From GitHub Releases (recommended)

Download the archive for your architecture from [GitHub Releases](https://github.com/yashikota/shippo/releases), then:

```bash
mkdir -p ~/bin ~/.config/systemd/user
tar xzf shippo_*_linux_amd64.tar.gz
mv shippo ~/bin/
cp shippo.service ~/.config/systemd/user/
sudo setcap cap_bpf,cap_net_admin,cap_sys_ptrace,cap_perfmon=ep ~/bin/shippo
```

`~/bin` must be on your `PATH`.

### From source (developers)

Build tools are required only when compiling from source:

```bash
sudo apt install clang llvm libbpf-dev   # if not already installed
git clone https://github.com/yashikota/shippo.git
cd shippo
aqua i          # installs task and other dev tools
task install    # build, setcap, install unit file
```

## Setup

These steps are the same whether you installed a release binary or built from source.

```bash
shippo init
sudo tailscale set --operator="$USER"
systemctl --user daemon-reload
systemctl --user enable --now shippo.service
```

If you built from source, `task enable` runs `task install` and the systemd commands above.

### `shippo init`

Initialization requires no root privileges. It lists currently listening localhost ports, lets you select candidate numbers, and accepts other ports or ranges. Enter `*` at either selection prompt to allow all localhost ports, including servers started later. Nothing is preselected. Review the allowlist and answer `y` to save it; Enter at the confirmation prompt cancels without changing the config.

Use `shippo --config <path> init` to choose another config location. Running init again replaces the allowlist only after confirmation. Unset `SHIPPO_PORTS` first if it is set, because that environment variable overrides the saved config.

`init` saves the allowlist only. It does not start the daemon or change Tailscale permissions.

## Usage

```bash
shippo                  # show help
shippo init             # choose allowed ports
shippo status           # show listening ports and allowlist
shippo add 3000         # add a port to the allowlist
shippo remove 3000      # remove a port from the allowlist
shippo daemon           # run in the foreground (needs setcap or root)
shippo once             # sync once and exit
```

Service management after install:

```bash
systemctl --user status shippo.service
journalctl --user -u shippo.service -f
systemctl --user disable --now shippo.service
```

When working from a source checkout, the same service actions are also available as `task status`, `task logs`, `task disable`, and `task enable`.

## Configuration

Allowed ports can be configured in three ways (in priority order):

### 1. Environment variable

```bash
SHIPPO_PORTS=3000,5173,8000 shippo daemon
```

### 2. Config file

`~/.config/shippo/config.json`:

```json
{
  "ports": ["3000", "5173", "8000"]
}
```

### 3. Default

If neither is set, no ports are allowed. Use `shippo add 3000` to allow a port.

## Capabilities

eBPF tracepoints require elevated privileges. Release installs and `task install` grant them with `setcap` on the binary:

- `CAP_BPF` – load BPF programs
- `CAP_PERFMON` – use ring buffer
- `CAP_NET_ADMIN` – network BPF operations
- `CAP_SYS_PTRACE` – attach tracepoints

Tailscale Serve is separate: grant operator permission with `sudo tailscale set --operator="$USER"`.

## Development

```bash
aqua i
task check
task lint
task test
```

See [TEST.md](TEST.md) for integration test requirements and coverage.

## License

MIT
