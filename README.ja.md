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
- Go 1.26.3+
- clang/llvm と libbpf headers（eBPF コンパイル用）
- Tailscale（インストール・認証済み）

## ビルド

```bash
sudo apt install clang llvm libbpf-dev   # 未インストールの場合
make build
```

`make build` は `gobee` で eBPF Go source を変換してから BPF object をコンパイルする。

## インストールと初期設定

```bash
aqua i
task install
~/bin/shippo init
sudo ~/bin/shippo
```

標準のインストール先は `~/bin`。PATH に含まれていれば `shippo init` で実行できる。
初期設定に root 権限は不要。現在 localhost で待受中のポートを候補として表示し、番号で選択する。
候補にないポートや範囲も追加できる。初期状態では何も選択しない。
最後に許可リストを表示し、`y` を入力した場合だけ設定を保存する。確認で Enter を押すと変更せず終了する。
再実行時も、保存を確認するまで既存設定は変更しない。
設定先を変更する場合は `shippo --config <path> init` を使う。
`SHIPPO_PORTS` は設定ファイルより優先されるため、設定されている場合は解除してから実行する。

デーモンの起動には root または BPF の権限が必要。`init` は設定を保存し、デーモンの起動や Tailscale の権限変更は行わない。

## 使い方

### デーモンモード

```bash
# 直接実行（root または capabilities が必要）
sudo ./shippo

# デバッグログ付きで実行
sudo ./shippo -v

# systemd user service としてインストール・有効化
make enable

# ログ確認
make logs

# 無効化
make disable
```

### CLI コマンド

```bash
# 公開を許可するポートを対話的に選んで保存
./shippo init

# 現在の状態を表示
./shippo status

# 許可リストにポートを追加（デーモンに即反映）
./shippo add 4000
./shippo add 8000-8999
./shippo add '*'

# 許可リストからポートを削除（デーモンに即反映）
./shippo remove 4000
./shippo remove 8000-8999

# 一回だけ同期して終了（デーモン化しない）
sudo ./shippo once

# 設定ファイルのパスを表示
./shippo config-path

# バージョン表示
./shippo --version

# ヘルプ
./shippo --help
```

### グローバルフラグ

| フラグ | 環境変数 | 説明 |
|--------|----------|------|
| `-v`, `--verbose` | | デバッグログを有効化 |
| `-c`, `--config` | `SHIPPO_CONFIG` | 設定ファイルのパスを指定 |
| `-l`, `--log-file` | `SHIPPO_LOG_FILE` | ログファイルのパスを指定（デフォルト: `/tmp/shippo.log`） |

ログは stderr とログファイルの両方に出力される。

## 設定

許可ポートは以下の優先順位で決定される：

### 1. 環境変数

```bash
SHIPPO_PORTS=3000,5173,8000-8999 sudo ./shippo
SHIPPO_PORTS='*' sudo ./shippo  # 全ポート許可
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

## プロジェクト構成

```
.
├── main.go                      # CLI エントリポイント
├── internal/daemon/
│   ├── daemon.go                # デーモン起動ロジック
│   ├── cli.go                   # status/add/remove 処理
│   ├── config.go                # 設定読み込み・保存・監視
│   ├── monitor.go               # eBPF ロード・イベント受信
│   ├── reconcile.go             # serve/unserve 判定
│   ├── serve.go                 # tailscale コマンド実行
│   └── bpf/src/shippo.go        # gobee eBPF Go コード
├── testserver/                  # テスト用 localhost サーバー
├── Makefile
└── shippo.service               # systemd user service
```

## 権限

eBPF tracepoint には特権が必要。systemd service では `AmbientCapabilities` で付与：

- `CAP_BPF` – BPF プログラムのロード
- `CAP_PERFMON` – ring buffer の使用
- `CAP_NET_ADMIN` – ネットワーク関連の BPF 操作

## テスト

`aqua i` の後、`task check`、`task lint`、`task test` で CI と同じ検証を実行する。
統合テストは Linux のネットワーク名前空間を分けて実際の eBPF と shippo を動かし、Tailscale コマンドだけをテスト用に置き換える。
必要な権限、検証範囲、実 Tailnet での接続確認は [test/README.md](test/README.md) を参照。
