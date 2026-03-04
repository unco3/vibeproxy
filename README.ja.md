# VibeProxy

[English](README.md)

バイブコーディング中にAPIキーが実行時ログ経由でLLMコンテキストに漏洩する問題を防ぐローカルプロキシツールです。エージェントにはダミートークンのみを渡し、プロキシが本物のAPIキーにスワップして転送します。

## 仕組み

```
エージェント (ダミートークン) → VibeProxy (localhost) → 外部API (本物のキー)
```

1. AIエージェントにはダミートークン（例: `vp-local-openai`）を渡す
2. VibeProxy がリクエストを受け取り、ダミートークンをシークレットプロバイダー（デフォルト: OSキーチェーン）から取得した本物のAPIキーにスワップ
3. 本物のキーはログ、コンテキスト、エージェントのメモリに一切表示されない

## 設計思想

**ステートレスなダミートークン:** `vp-local-openai` のようなトークンは、サービス名から決定論的に導出される識別マーカーであり、秘密情報ではありません。トークンの役割は「このリクエストはOpenAIサービス向け — 本物のキーを取得せよ」とVibeProxyに伝えることです。プロキシは各開発者のローカルマシンで独自のキーチェーンと共に稼働するため、ユーザーごとのトークン生成やファイルベースの状態管理（`.vibelocal`）は不要です。

**プラガブルなシークレットバックエンド:** 本物のAPIキーは `secret.Provider` インターフェースを通じて管理されます。デフォルトバックエンドはOSキーチェーン（macOS Keychain / Linux Secret Service / Windows Credential Manager）です。環境変数、1Password CLI、AWS Secrets Manager等の代替バックエンドは、単一のインターフェースを実装するだけで追加可能です。

## スコープと非目標

VibeProxyは意図的に**軽量・単一目的のローカルプロキシ**として設計されています。以下の設計判断は意図的かつ最終的なものです。

**VibeProxyがやること:**
- localhost上でダミートークンを本物のAPIキーにスワップ
- YAML設定（`auth_header` + `auth_scheme`）によるHTTPヘッダーベースの任意の認証プロバイダーのサポート
- プラガブルなシークレットバックエンド（`secret.Provider` インターフェース）
- オプショナルなOpenAI互換ゲートウェイ（モデルベースのルーティング）
- JSON監査ログとプロバイダー互換のエラーレスポンス

**VibeProxyが意図的にやらないこと:**
- **ユーザーごとのトークン生成（`.vibelocal`）:** ダミートークンは決定論的なマーカー（`vp-local-{service}`）であり、秘密情報ではない。各開発者が自分のキーチェーンと共にローカルプロキシを実行するため、ユーザー間のトークン区別は不要な複雑さを追加するだけ。
- **プラグイン/インターフェースベースのプロバイダー抽象化:** 現在の `HeaderProvider` + YAML設定で主要なLLMプロバイダー（OpenAI、Anthropic、Google、Mistral等）をすべてカバー。特殊な認証方式（例: BedrockのAWS V4署名）は実際の需要が発生した時点で対応する。
- **OpenTelemetry / 可観測性ダッシュボード:** ローカルプロキシに分散トレーシングは不要。`slog` + JSON監査ログで十分。エンタープライズ需要が出た場合は `slog` からOTelへのブリッジで対応可能。
- **フレームワークベースのミドルウェア/ルーティング:** Go標準のミドルウェアチェインパターン（`func(http.Handler) http.Handler`）はイディオマティックで可読性が高く、依存関係ゼロ。

## クイックスタート

```bash
# クローン & ビルド
git clone https://github.com/unco3/vibeproxy.git
cd vibeproxy
go build -o vibe .

# プロジェクト設定を初期化
./vibe init

# 本物のAPIキーをOSキーチェーンに保存
./vibe auth login openai
./vibe auth login anthropic

# プロキシを起動
./vibe start

# ステータス確認
./vibe status

# 監査ログを表示
./vibe logs

# プロキシを停止
./vibe stop
```

## 設定ファイル

### vibeproxy.yaml (チーム共有、Gitコミット対象)

```yaml
services:
  openai:
    target: https://api.openai.com
    auth_header: Authorization
    auth_scheme: Bearer
    allowed_paths:
      - /v1/chat/completions
      - /v1/embeddings
      - /v1/responses
    rate_limit:
      requests_per_minute: 60

  anthropic:
    target: https://api.anthropic.com
    auth_header: x-api-key
    auth_scheme: ""
    allowed_paths:
      - /v1/messages
    rate_limit:
      requests_per_minute: 60

listen: 127.0.0.1:8080

timeouts:
  read_seconds: 30       # クライアントリクエスト読み取りタイムアウト
  write_seconds: 120     # レスポンス書き込みタイムアウト（LLMストリーミング用に長めに設定）
  upstream_seconds: 90   # アップストリームAPIレスポンスヘッダータイムアウト

cors:
  enabled: false
  allowed_origins:
    - http://localhost:3000

# オプション: secret_backend: env  (デフォルト: keychain)

# オプション: OpenAI互換ゲートウェイ
# gateway:
#   enabled: true
#   paths:
#     - /v1/chat/completions
#     - /v1/embeddings
#   models:
#     gpt-: openai
#     claude-: anthropic
```

### .env (エージェント向け参照、gitignore対象)

`vibe init` で自動生成されます。エージェントの環境変数をここに向けてください。

```bash
VIBEPROXY_URL=http://127.0.0.1:8080
OPENAI_API_KEY=vp-local-openai
ANTHROPIC_API_KEY=vp-local-anthropic
```

## リクエストフロー

```
リクエスト受信
  → localhost検証
  → トークン抽出 (Authorization / x-api-key ヘッダー)
  → ルート解決 (vp-local-{service} プレフィックスからサービス名を特定)
  → パスホワイトリスト検証
  → レート制限検証
  → シークレット取得 (ダミー → シークレットプロバイダーから本物のキー)
  → httputil.ReverseProxy → 外部API
  → レスポンス → 監査ログ記録
```

## アーキテクチャ

| 機能 | 詳細 |
|---|---|
| **トークンスワップ** | `Authorization` または `x-api-key` ヘッダーからダミートークンを抽出し、シークレットプロバイダーから本物のキーを取得してリクエストを転送 |
| **localhost限定** | `127.0.0.1` にのみバインドし、それ以外のアドレスは拒否 |
| **パスホワイトリスト** | 設定されたAPIパスのみプロキシ対象。それ以外は403を返却 |
| **レート制限** | サービスごとのインメモリスライディングウィンドウカウンタ（再起動でリセット） |
| **タイムアウト制御** | read/write/upstreamタイムアウトを設定可能。遅延やハングへの耐性 |
| **CORS** | ブラウザベースのエージェントUI向けのオプショナルCORSサポート（デフォルト無効） |
| **エラーフォーマット** | プロバイダー互換のエラーレスポンス（OpenAI/Anthropic SDK形式） |
| **ゲートウェイ** | オプショナルなOpenAI互換ユニバーサルAPI — モデルプレフィックスで任意のプロバイダーにルーティング |
| **監査ログ** | `~/.vibeproxy/audit.log` にJSON Lines形式で記録（タイムスタンプ、メソッド、パス、ステータスコード、所要時間。シークレットは記録しない） |
| **デーモンモード** | self-re-execパターンによるバックグラウンド実行。PIDファイルは `~/.vibeproxy/vibeproxy.pid` |

## CLIリファレンス

| コマンド | 説明 |
|---|---|
| `vibe init` | `vibeproxy.yaml`、`.env` を生成し、`.gitignore` を更新 |
| `vibe auth login <provider>` | APIキーをシークレットプロバイダー経由で保存 |
| `vibe start` | プロキシをバックグラウンドデーモンとして起動 |
| `vibe start --foreground` | プロキシをフォアグラウンドで起動 |
| `vibe stop` | 実行中のプロキシを停止 |
| `vibe status` | プロキシの実行状態を表示 |
| `vibe logs [-n 20]` | 最近の監査ログエントリを表示 |
| `vibe service install` | OSサービスとして登録（macOS launchd。ブート時自動起動） |
| `vibe service uninstall` | OSサービスから削除 |

## セキュリティモデル

- 本物のAPIキーはプラガブルなシークレットプロバイダー（デフォルト: OSキーチェーン）を通じて管理
- キーはオンデマンドでメモリにロードされ、ディスクやログには一切書き込まれない
- ダミートークン (`vp-local-*`) は静的な識別マーカーであり、秘密情報ではない — ログ、`.env`ファイル、LLMコンテキストに表示されても安全
- プロキシはlocalhostにのみバインドされ、ネットワークに露出しない
- パスホワイトリストにより、意図しないAPIエンドポイントへのアクセスを防止

## 必要環境

- Go 1.21+
- macOS / Linux / Windows（[go-keyring](https://github.com/zalando/go-keyring) 経由）
