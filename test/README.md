# Testing shippo

Install Go (the version in `go.mod` or newer), clang, LLVM, libbpf headers,
Docker, and [aqua](https://aquaproj.github.io/docs/install). Then:

```sh
aqua i
task check
task lint
task test
```

`task test:unit` runs the allowlist, `/proc` parsing, publication lifecycle,
failure recovery, command arguments, and URL state tests with Go's race detector.
It needs no root privileges or Tailscale account.

`task test:integration` builds the real gobee BPF objects and race-enabled daemon
and test binaries, then runs them in a disposable Ubuntu container. It checks:

- BPF verifier acceptance and attachment to `sock/inet_sock_set_state`.
- Actual TCP LISTEN start/stop events on IPv4 and IPv6, including address and
  port decoding and rejection of wildcard addresses.
- Monitor shutdown while the event consumer is blocked.
- Daemon startup with an existing listener, live start/stop detection, denied
  ports, atomic config replacement, an empty allowlist, and SIGTERM cleanup.

Docker uses `--privileged` and a read-only mount of `/sys/kernel/tracing` because
the tests load and attach BPF programs. It uses `--network none`, its own network
namespace, and temporary configuration/state files. It does not mount the host's
Tailscale socket, home directory, or configuration. Run it on a Linux host or VM
where tracefs is mounted. Missing BPF permissions or tracepoints fail the test;
they are never treated as a successful skip.

The kernel is real, but the `tailscale` executable is a test double that records
commands and can return permission failures. These tests do **not** prove
Tailscale authentication, certificate issuance, or HTTPS connectivity from a peer.
Those require a separate real Tailnet test with HTTPS enabled and CI credentials.
Headscale currently cannot replace that HTTPS certificate flow.

CI runs these same commands on GitHub-hosted Ubuntu 22.04 and 24.04 VMs, whose
kernels are shared by their test containers. This tests the kernels supplied by
those runners, not every kernel supported by shippo. Both amd64 and arm64 binaries
are compiled; runtime integration currently runs on amd64.

For an actual installation, authenticate Tailscale, enable HTTPS certificates,
and authorize the daemon's user to manage Serve:

```sh
sudo tailscale set --operator="$USER"
```

This is separate from the BPF capabilities. `Access denied: serve config denied`
means the Tailscale operator permission is missing. A running systemd unit alone
does not establish that publication works: check `tailscale serve status` and
request the URL from another Tailnet device. After fixing a publication failure,
a new LISTEN event or configuration change triggers another attempt.
