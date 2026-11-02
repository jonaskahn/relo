<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ 軽量 • 高速 • 堅牢 ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>設定を変えずに、harness agent をあらゆるモデル・あらゆるプロバイダーにつなげます。</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ コンソール

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Relo のデモ</sub>
</div>

| ダッシュボード                                                                                                               | 接続                                                                                                               | Agent Harness                                                                                                          |
| ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="ダッシュボード" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="接続" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="グループ" width="320"> |

## ▓ インストール

どれか 1 つを選んでください。どのインストーラーも 2 つ目の Relo を重ね入れしません。

- Linux & macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| チャネル     | コマンド                                                                 |
| ------------ | ------------------------------------------------------------------------ |
| スクリプト   | 上の 2 行。`install.sh --help` で全オプションを表示                      |
| Homebrew     | `brew install jonaskahn/apps/relo`                                       |
| npm          | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts` (Node 22+)        |
| macOS アプリ | `Relo-<version>-universal.zip`                                           |
| Linux        | `Relo-Setup-<version>-amd64.deb` または `Relo-Setup-<version>-amd64.rpm` |
| Windows      | `Relo-Setup-<version>-amd64.exe`                                         |

セットアップファイルは[最新リリース](https://github.com/jonaskahn/relo/releases/latest)から取得し、その後アプリが
自動更新します。スクリプトは daemon バイナリをインストールします。ダウンロードを `checksums.txt` で検証し、Relo が
使うポートを解放してから daemon を起動します。npm インストールも同じ 13 MB のバイナリを取得し、各手順を出力します。
進行バーには速度と残り時間が出ます。`--foreground-scripts` を付けると、その出力は隠れずインストールコマンドの下に
残ります。

Windows では setup インストーラーと npm インストールがデスクトップに「Relo」ショートカットを追加します。この
ショートカットはターミナルを開かずに `relo daemon restart --force` を実行します。

## ▓ 起動

```shell
relo daemon start                 # コンソールは http://127.0.0.1:10101
```

`daemon start` はプロキシをバックグラウンドで動かし、デスクトップセッションがあればトレイアイコンを表示します。
`daemon run` は同じプロセスをフォアグラウンドに残します。ターミナル、ログイン項目、Relo.app はこの方式です。

`daemon run|start|restart --silent` はブラウザでダッシュボードを開きません。ログイン項目はこうして起動します。

ループバックのみです。`[admin] login = true` を指定すると、`$RELO_HOME/admin-token` を使うサインインが前に付きます。

## ▓ ポートとプレフィックス

| ポート  | 話すもの                               |
| ------- | -------------------------------------- |
| `10101` | コンソール + 管理 API + データプレーン |

| プレフィックス      | 話すもの              |
| ------------------- | --------------------- |
| `/inference`        | OpenAI 互換 (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)   |

クライアントキーはデータプレーンへ、admin トークンはコンソールへ送ります。プロバイダーの認証情報は上流にだけ送られます。

## ▓ コマンド

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` と `--lang` は 1 回の実行だけ上書きします。それ以外（接続、キー、ルート）はコンソールのページです。

`daemon stop --force` は daemon を停止し、Relo が使うポートを解放します。インストーラーがバイナリを置き換える前に
実行するのはこれです。ほかのプログラムが持つポートは終了させず、報告するだけです。

## ▓ アンインストール

```console
$ relo uninstall            # データを消すか尋ねてからプログラムを削除します
$ relo uninstall --keep-data # Relo を停止し、ログイン項目を解除し、データは残します
$ relo uninstall --wipe      # 上と同じうえで $RELO_HOME とそのキーチェーン項目も削除します
```

`relo uninstall` は実行中の daemon を停止し、設定されたすべてのポートを解放し、ログイン時の起動を切り、データの扱いを
決め、Relo のインストール方法を判定して同じ方法で削除します。Windows setup のアンインストーラー、`npm uninstall -g`、
`brew uninstall`、`Relo.app` の削除、スクリプトで入れたバイナリの削除のいずれかです。`.deb` や `.rpm` の場合は、
権限が必要なコマンドを実行せずに表示します。バイナリが動かないときも同じ判定が使えます。

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

各方法が残すデータ:

| 方法                               | データ                               |
| ---------------------------------- | ------------------------------------ |
| `relo uninstall`                   | 尋ねる。`y` で削除、それ以外は保持   |
| Windows: 設定 → アプリ             | ダイアログで尋ねる。「いいえ」で保持 |
| `apt remove relo`                  | 保持（`apt purge relo` は削除）      |
| `rpm -e relo`                      | 保持                                 |
| `npm uninstall -g @jonaskahn/relo` | 保持                                 |
| `brew uninstall relo`              | 保持                                 |

更新や再インストールは常にデータを残し、先に実行中のサービスを止めます。動作中のインストールに上書きしても失うものは
ありません。

## ▓ 設定

`$RELO_HOME`（`~/.relo`）が唯一の環境変数です。設定は `config.toml` にあります。
[`config.example.toml`](config.example.toml) を参照してください。

シークレットは `$RELO_HOME/secrets.enc` にあり、`$RELO_HOME/secret.key` で封をします。
`[secrets] keychain = true` なら OS のキーチェーンを使います。

## ▓ 開発

```console
$ make build          # コンソール + daemon → ./relo
$ make seed           # なければ models.dev のビルドシードを取得
$ make dev-setup      # .dev/relo を書き、~/.relo には触れません
$ make ci
```

Go 1.26+、Node 22+。CGO 不要。詳しくは [CONTRIBUTING.md](CONTRIBUTING.md) と [docs/](docs/README.md)、脆弱性の報告は
[SECURITY.md](SECURITY.md)、同梱ソフトウェアのライセンスは [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を
参照してください。

## ▓ ライセンス

Relo は [GNU Affero General Public License v3.0](LICENSE) で提供されます。同梱のサードパーティ製ソフトウェアは
それぞれのライセンスを保ちます。[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を参照してください。
