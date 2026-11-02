<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ 轻巧 • 快速 • 可靠 ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>让 harness agent 接入任意模型、任意提供商，无需改动配置。</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ 控制台

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Relo 使用演示</sub>
</div>

| 仪表盘                                                                                                               | 连接                                                                                                               | Agent Harness                                                                                                      |
| -------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="仪表盘" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="连接" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="分组" width="320"> |

## ▓ 安装

任选一种。所有安装程序都不会叠加第二个 Relo。

- Linux 与 macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| 渠道       | 命令                                                                 |
| ---------- | -------------------------------------------------------------------- |
| 脚本       | 上面的两行；`install.sh --help` 会列出所有参数                       |
| Homebrew   | `brew install jonaskahn/apps/relo`                                   |
| npm        | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts`（Node 22+）   |
| macOS 应用 | `Relo-<version>-universal.zip`                                       |
| Linux      | `Relo-Setup-<version>-amd64.deb` 或 `Relo-Setup-<version>-amd64.rpm` |
| Windows    | `Relo-Setup-<version>-amd64.exe`                                     |

安装文件来自[最新发布](https://github.com/jonaskahn/relo/releases/latest)，装好后应用会自行更新。脚本安装的是 daemon
二进制：它用 `checksums.txt` 校验下载内容，释放 Relo 占用的端口，然后启动 daemon。npm 安装会下载同一个 13 MB
二进制并逐条打印步骤，进度条显示速率与剩余时间；`--foreground-scripts` 会让这些输出留在安装命令下方，而不是被隐藏。

在 Windows 上，setup 安装程序和 npm 安装都会添加桌面“Relo”快捷方式，直接运行 `relo daemon restart --force`，不打开终端。

## ▓ 启动

```shell
relo daemon start                 # 控制台位于 http://127.0.0.1:10101
```

`daemon start` 在后台运行代理，并在有桌面会话时显示托盘图标。`daemon run` 让同一进程留在前台，终端、登录项和
Relo.app 用的就是它。

`daemon run|start|restart --silent` 不会在浏览器中打开仪表盘，登录项正是这样启动的。

仅限回环地址。`[admin] login = true` 会在前面加一道登录，使用 `$RELO_HOME/admin-token`。

## ▓ 端口与路径

| 端口    | 协议                         |
| ------- | ---------------------------- |
| `10101` | 控制台 + 管理 API + 数据平面 |

| 前缀                | 协议                   |
| ------------------- | ---------------------- |
| `/inference`        | OpenAI 兼容（`/v1/*`） |
| `/inference-claude` | Anthropic（`/v1/*`）   |

客户端密钥走数据平面，admin token 走控制台。提供商凭据只向上游发送。

## ▓ 命令

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` 和 `--lang` 只覆盖当次运行。其余内容（连接、密钥、路由）都在控制台页面里。

`daemon stop --force` 会停止 daemon 并释放 Relo 占用的端口，安装程序替换二进制前正是执行它。若端口被其他程序占用，
只会报告，不会结束该程序。

## ▓ 卸载

```console
$ relo uninstall            # 询问是否保留数据，然后移除程序
$ relo uninstall --keep-data # 停止 Relo，注销登录项，保留数据
$ relo uninstall --wipe      # 同上，并删除 $RELO_HOME 及其钥匙串条目
```

`relo uninstall` 会停止正在运行的 daemon、释放所有已配置的端口、关闭开机自启、按你的选择处理数据、识别 Relo 的安装方式，
并用同样的方式卸载：Windows setup 卸载程序、`npm uninstall -g`、`brew uninstall`、删除 `Relo.app`，或删除脚本安装的
二进制。`.deb` 或 `.rpm` 安装会打印需要提权的命令，而不是直接执行。二进制已无法运行时，也可以用同样的识别方式：

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

各卸载方式对数据的处理：

| 方式                               | 数据                          |
| ---------------------------------- | ----------------------------- |
| `relo uninstall`                   | 询问；`y` 删除，其他保留      |
| Windows：设置 → 应用               | 弹窗询问；选“否”保留          |
| `apt remove relo`                  | 保留（`apt purge relo` 删除） |
| `rpm -e relo`                      | 保留                          |
| `npm uninstall -g @jonaskahn/relo` | 保留                          |
| `brew uninstall relo`              | 保留                          |

更新或重装始终保留数据，并先停止正在运行的服务，因此覆盖安装不会有任何丢失。

## ▓ 配置

`$RELO_HOME`（`~/.relo`）是唯一的环境变量。设置保存在 `config.toml`。参见
[`config.example.toml`](config.example.toml)。

机密保存在 `$RELO_HOME/secrets.enc`，由 `$RELO_HOME/secret.key` 加封。`[secrets] keychain = true` 改用操作系统钥匙串。

## ▓ 开发

```console
$ make build          # 控制台 + daemon → ./relo
$ make seed           # 缺少时下载 models.dev 构建种子
$ make dev-setup      # 写入 .dev/relo，不动 ~/.relo
$ make ci
```

Go 1.26+、Node 22+，无需 CGO。其余内容见 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [docs/](docs/README.md)；报告漏洞请见
[SECURITY.md](SECURITY.md)，随附软件的许可证见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## ▓ 许可证

Relo 采用 [GNU Affero General Public License v3.0](LICENSE) 许可。随附的第三方软件保留各自的许可证；参见
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
