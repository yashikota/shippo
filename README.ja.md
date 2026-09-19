# shippo

eBPF を使ってイベント駆動で localhost の LISTEN ポートを検出し、`tailscale serve` で自動公開するデーモン

## 必要なもの

- Linux kernel 5.8+
- Tailscale（インストール・認証済み）

## インストール

```bash
case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported: $(uname -m)" >&2; exit 1 ;;
esac
VERSION=$(
  curl -fsSL https://api.github.com/repos/yashikota/shippo/releases/latest \
    | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p'
)
curl -fsSL "https://github.com/yashikota/shippo/releases/download/${VERSION}/shippo_${VERSION}_linux_${ARCH}.tar.gz" \
  | tar xz
sudo install -m 755 shippo /usr/local/bin/shippo
sudo setcap cap_bpf,cap_net_admin,cap_sys_ptrace,cap_perfmon=ep /usr/local/bin/shippo
mkdir -p ~/.config/systemd/user
cp shippo.service ~/.config/systemd/user/
```

## セットアップ

```bash
shippo init
sudo tailscale set --operator="$USER"
systemctl --user daemon-reload
systemctl --user enable --now shippo.service
```

`shippo init` で公開を許可するポートを選ぶ。root 権限は不要。
`*` を指定すると localhost の全ポートを許可する。最後に `y` で保存する。

動作確認:

```bash
shippo status
tailscale serve status
```

localhost で許可したポートのサーバーを起動すると、`https://<マシン名>.<tailnet>:<PORT>` で Tailnet 内からアクセスできる。

## 仕組み

```txt
┌─────────────────────────────────────────────────┐
│  eBPF tracepoint: sock/inet_sock_set_state      │
│  → LISTEN 開始・終了を ring buffer で即時通知   │
└──────────────────┬──────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────┐
│  shippo デーモン                              　│
│  - LISTEN 開始 → tailscale serve --https=PORT 　│
│  - LISTEN 終了 → tailscale serve off          　│
└─────────────────────────────────────────────────┘
```

- ポーリングなし。eBPF tracepoint によるイベント駆動
- 許可リストにあるポートだけ公開
- URL は `https://<マシン名>.<tailnet>:<PORT>`

## 使い方

### コマンド

```bash
shippo init             # 許可ポートを対話的に設定
shippo status           # 待受ポートと許可リストを表示
shippo add 4000         # 許可リストに追加
shippo remove 4000      # 許可リストから削除
shippo daemon           # フォアグラウンド実行（通常は systemd を使う）
```

### サービス管理

```bash
systemctl --user status shippo.service
journalctl --user -u shippo.service -f
systemctl --user disable --now shippo.service
```

## 設定

許可ポートは次の優先順位で決まる。

1. 環境変数 `SHIPPO_PORTS`（例: `SHIPPO_PORTS=3000,8000-8999 shippo daemon`）
2. 設定ファイル `~/.config/shippo/config.json`

```json
{
  "ports": ["3000", "5173", "8000-8999"]
}
```

## 権限

eBPF tracepoint には特権が必要。Release インストールと `task install` はバイナリに `setcap` で付与する。

- `CAP_BPF` – BPF プログラムのロード
- `CAP_PERFMON` – ring buffer の使用
- `CAP_NET_ADMIN` – ネットワーク関連の BPF 操作
- `CAP_SYS_PTRACE` – tracepoint の attach

Tailscale Serve は別途 `sudo tailscale set --operator="$USER"` が必要。

## 開発

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

詳細は [TEST.md](TEST.md) を参照。
