# shippo

eBPF を使ってイベント駆動で localhost の LISTEN ポートを検出し、`tailscale serve` で自動公開するデーモン

## 必要なもの

- Linux kernel 5.8+
- Tailscale（インストール・認証済み）

## インストール

[GitHub Releases](https://github.com/yashikota/shippo/releases) からダウンロード  

```bash
mkdir -p ~/bin ~/.config/systemd/user
tar xzf shippo_*_linux_amd64.tar.gz
mv shippo ~/bin/
cp shippo.service ~/.config/systemd/user/
sudo setcap cap_bpf,cap_net_admin,cap_sys_ptrace,cap_perfmon=ep ~/bin/shippo
```

## セットアップ

```bash
shippo init
sudo tailscale set --operator="$USER"
systemctl --user daemon-reload
systemctl --user enable --now shippo.service
```

ソースから入れた場合は `task enable` で `task install` と上記 systemd 有効化までまとめて実行できる  

## 仕組み

```txt
┌─────────────────────────────────────────────────┐
│  eBPF tracepoint: sock/inet_sock_set_state      │
│  → LISTEN 開始・終了を ring buffer で即時通知      │
└──────────────────┬──────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────┐
│  shippo デーモン                              　　│
│  - LISTEN 開始 → tailscale serve --https=PORT  　│
│  - LISTEN 終了 → tailscale serve off           　│
└─────────────────────────────────────────────────┘
```

- ポーリングなし。eBPF tracepointによるイベント駆動
- 許可リストにあるポートだけ公開
- URL は `https://<マシン名>.<tailnet>:<PORT>` で公開

## 使い方

### コマンド

```bash
shippo init             # 初期設定を対話的に設定
shippo status           # 待受ポートと許可リストを表示
shippo add 4000         # 指定ポートを許可リストに追加
shippo remove 4000      # 指定ポートを許可リストから削除
shippo daemon           # フォアグラウンド実行（setcap または root が必要）
shippo once             # 1 回だけ同期して終了
```

### サービス管理

```bash
systemctl --user status shippo.service
journalctl --user -u shippo.service -f
systemctl --user disable --now shippo.service
```

許可ポートは以下の優先順位で決定される。

### 1. 環境変数

```bash
SHIPPO_PORTS=3000,5173,8000-8999 shippo daemon
SHIPPO_PORTS='*' shippo daemon
```

### 2. 設定ファイル

デフォルトパス: `~/.config/shippo/config.json`

```json
{
  "ports": ["3000", "5173", "8000-8999"]
}
```

### 3. デフォルト

どちらも未設定の場合、許可ポートなし（`shippo add` で追加する）。

### ポート指定形式

| 形式 | 例 | 説明 |
|------|-----|------|
| 単一ポート | `3000` | 指定したポートのみ |
| レンジ | `8000-8999` | 範囲内の全ポート |
| ワイルドカード | `*` | localhost の全ポート |

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
task install
task check
task lint
task test
```
