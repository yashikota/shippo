# shippo

eBPF を使ってイベント駆動で localhost の LISTEN ポートを検出し、`tailscale serve` で自動公開するデーモン

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

- ポーリングなし。eBPF tracepoint による純粋なイベント駆動。
- 許可リストにあるポートだけ公開。DB や管理用サーバーが意図せず公開されることはない。
- URL は `https://<マシン名>.<tailnet>:<PORT>` で公開される。
- 設定ファイル変更は fsnotify で即座に反映される。

## 必要なもの

- Linux kernel 5.8+
- Tailscale（インストール・認証済み）

## インストール

### GitHub Releases から（推奨）

[GitHub Releases](https://github.com/yashikota/shippo/releases) からアーキテクチャに合ったアーカイブを取得する。

```bash
mkdir -p ~/bin ~/.config/systemd/user
tar xzf shippo_*_linux_amd64.tar.gz
mv shippo ~/bin/
cp shippo.service ~/.config/systemd/user/
sudo setcap cap_bpf,cap_net_admin,cap_sys_ptrace,cap_perfmon=ep ~/bin/shippo
```

`~/bin` は `PATH` に含まれている必要がある。

### ソースから（開発者向け）

ソースビルド時だけ clang などが必要。

```bash
sudo apt install clang llvm libbpf-dev   # 未インストールの場合
git clone https://github.com/yashikota/shippo.git
cd shippo
aqua i          # task など開発ツールを導入
task install    # ビルド、setcap、unit ファイル配置
```

## セットアップ

Release バイナリでもソースビルドでも手順は同じ。

```bash
shippo init
sudo tailscale set --operator="$USER"
systemctl --user daemon-reload
systemctl --user enable --now shippo.service
```

ソースから入れた場合は `task enable` で `task install` と上記 systemd 有効化までまとめて実行できる。

### `shippo init`

初期設定に root 権限は不要。現在 localhost で待受中のポートを候補として表示し、番号で選択する。候補にないポートや範囲も追加できる。候補選択または追加入力で `*` を指定すると、後から起動するサーバーも含め localhost の全ポートを許可する。初期状態では何も選択しない。最後に許可リストを表示し、`y` を入力した場合だけ設定を保存する。確認で Enter を押すと変更せず終了する。

再実行時も、保存を確認するまで既存設定は変更しない。設定先を変更する場合は `shippo --config <path> init` を使う。`SHIPPO_PORTS` は設定ファイルより優先されるため、設定されている場合は解除してから実行する。

`init` は許可リストの保存のみ行う。デーモンの起動や Tailscale 権限の変更は行わない。

## 使い方

### よく使うコマンド

```bash
shippo                  # ヘルプ表示
shippo init             # 許可ポートを対話的に設定
shippo status           # 待受ポートと許可リストを表示
shippo add 4000         # 許可リストに追加（デーモンに即反映）
shippo remove 4000      # 許可リストから削除
shippo daemon           # フォアグラウンド実行（setcap または root が必要）
shippo once             # 1 回だけ同期して終了
shippo config-path      # 設定ファイルのパスを表示
shippo --version
```

### サービス管理

```bash
systemctl --user status shippo.service
journalctl --user -u shippo.service -f
systemctl --user disable --now shippo.service
```

ソース checkout から作業している場合は `task status`、`task logs`、`task disable`、`task enable` でも同じ操作ができる。

### グローバルフラグ

| フラグ | 環境変数 | 説明 |
|--------|----------|------|
| `-v`, `--verbose` | | デバッグログを有効化 |
| `-c`, `--config` | `SHIPPO_CONFIG` | 設定ファイルのパスを指定 |
| `-l`, `--log-file` | `SHIPPO_LOG_FILE` | ログファイルのパス（デフォルト: `/tmp/shippo.log`） |

ログは stderr とログファイルの両方に出力される。

## 設定

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

設定ファイルは fsnotify で監視されており、`shippo add/remove` や手動編集の変更がデーモンに即座に反映される。

## 権限

eBPF tracepoint には特権が必要。Release インストールと `task install` はバイナリに `setcap` で付与する。

- `CAP_BPF` – BPF プログラムのロード
- `CAP_PERFMON` – ring buffer の使用
- `CAP_NET_ADMIN` – ネットワーク関連の BPF 操作
- `CAP_SYS_PTRACE` – tracepoint の attach

Tailscale Serve は別途 `sudo tailscale set --operator="$USER"` が必要。

## 開発

```bash
aqua i
task check
task lint
task test
```

統合テストの前提、検証範囲、実 Tailnet での確認手順は [TEST.md](TEST.md) を参照。

## プロジェクト構成

```
.
├── main.go
├── Taskfile.yaml                # ビルド・インストール・テスト
├── internal/daemon/
│   ├── daemon.go
│   ├── monitor.go               # eBPF ロード・イベント受信
│   ├── reconcile.go
│   ├── serve.go
│   └── bpf/src/shippo.go        # gobee eBPF Go コード
└── shippo.service
```
