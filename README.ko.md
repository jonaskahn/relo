<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ 가볍게 • 빠르게 • 안정적으로 ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>설정을 건드리지 않고 harness agent를 어떤 모델, 어떤 제공업체와도 연결하세요.</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ 콘솔

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Relo 사용 데모</sub>
</div>

| 대시보드                                                                                                               | 연결                                                                                                               | Agent Harness                                                                                                      |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="대시보드" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="연결" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="그룹" width="320"> |

## ▓ 설치

하나만 고르세요. 모든 설치 프로그램은 두 번째 Relo를 덧씌우지 않습니다.

- Linux & macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| 채널     | 명령                                                                   |
| -------- | ---------------------------------------------------------------------- |
| 스크립트 | 위의 두 줄; `install.sh --help`가 모든 플래그를 보여줍니다             |
| Homebrew | `brew install jonaskahn/apps/relo`                                     |
| npm      | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts` (Node 22+)      |
| macOS 앱 | `Relo-<version>-universal.zip`                                         |
| Linux    | `Relo-Setup-<version>-amd64.deb` 또는 `Relo-Setup-<version>-amd64.rpm` |
| Windows  | `Relo-Setup-<version>-amd64.exe`                                       |

설치 파일은 [최신 릴리스](https://github.com/jonaskahn/relo/releases/latest)에서 받고, 설치 후에는 앱이 스스로
업데이트합니다. 스크립트는 daemon 바이너리를 설치합니다. `checksums.txt`로 다운로드를 검증하고, Relo가 쓰는 포트를
비운 뒤 daemon을 시작합니다. npm 설치도 같은 13 MB 바이너리를 내려받으며 단계마다 출력하고, 속도와 남은 시간을
보여주는 진행 표시줄을 띄웁니다. `--foreground-scripts`를 쓰면 이 출력이 숨겨지지 않고 설치 명령 아래에 남습니다.

Windows에서는 setup 설치 프로그램과 npm 설치가 데스크톱에 "Relo" 바로가기를 추가하며, 이 바로가기는 터미널을 열지
않고 `relo daemon restart --force`를 실행합니다.

## ▓ 시작

```shell
relo daemon start                 # 콘솔: http://127.0.0.1:10101
```

`daemon start`는 프록시를 백그라운드에서 실행하고, 데스크톱 세션이 있으면 트레이 아이콘을 표시합니다. `daemon run`은
같은 프로세스를 포그라운드에 두며, 터미널·로그인 항목·Relo.app이 쓰는 방식입니다.

`daemon run|start|restart --silent`는 브라우저에서 대시보드를 열지 않습니다. 로그인 항목은 이렇게 시작합니다.

루프백 전용입니다. `[admin] login = true`를 두면 `$RELO_HOME/admin-token`을 쓰는 로그인 화면이 앞에 붙습니다.

## ▓ 포트와 접두사

| 포트    | 말하는 것                       |
| ------- | ------------------------------- |
| `10101` | 콘솔 + 관리 API + 데이터 플레인 |

| 접두사              | 말하는 것             |
| ------------------- | --------------------- |
| `/inference`        | OpenAI 호환 (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)   |

클라이언트 키는 데이터 플레인으로, admin 토큰은 콘솔로 갑니다. 제공업체 자격 증명은 업스트림으로만 나갑니다.

## ▓ 명령

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home`과 `--lang`은 한 번의 실행에만 적용됩니다. 나머지(연결, 키, 라우트)는 모두 콘솔 페이지입니다.

`daemon stop --force`는 daemon을 멈추고 Relo가 쓰는 포트를 비웁니다. 설치 프로그램이 바이너리를 교체하기 전에
실행하는 명령입니다. 다른 프로그램이 잡고 있는 포트는 종료하지 않고 알려주기만 합니다.

## ▓ 제거

```console
$ relo uninstall            # 데이터를 지울지 물은 뒤 프로그램을 제거합니다
$ relo uninstall --keep-data # Relo를 멈추고 로그인 항목을 해제하며 데이터는 남깁니다
$ relo uninstall --wipe      # 위와 같고 $RELO_HOME과 그 키체인 항목까지 삭제합니다
```

`relo uninstall`은 실행 중인 daemon을 멈추고, 설정된 모든 포트를 비우고, 로그인 시 자동 시작을 끄고, 데이터를 결정하고,
Relo가 어떻게 설치되었는지 감지해 같은 방식으로 제거합니다. Windows setup 제거 프로그램, `npm uninstall -g`,
`brew uninstall`, `Relo.app` 삭제, 또는 스크립트로 설치한 바이너리 삭제입니다. `.deb`나 `.rpm` 설치는 권한 있는 명령을
실행하지 않고 출력합니다. 바이너리가 실행되지 않아도 같은 감지를 쓸 수 있습니다.

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

방식별로 남기는 데이터:

| 방식                               | 데이터                             |
| ---------------------------------- | ---------------------------------- |
| `relo uninstall`                   | 물어봄; `y`면 삭제, 그 외는 유지   |
| Windows: 설정 → 앱                 | 대화상자에서 물어봄; 아니요면 유지 |
| `apt remove relo`                  | 유지 (`apt purge relo`는 삭제)     |
| `rpm -e relo`                      | 유지                               |
| `npm uninstall -g @jonaskahn/relo` | 유지                               |
| `brew uninstall relo`              | 유지                               |

업데이트나 재설치는 항상 데이터를 남기고 실행 중인 서비스를 먼저 멈추므로, 잘 동작하는 설치 위에 덮어 설치해도 잃는
것이 없습니다.

## ▓ 설정

`$RELO_HOME`(`~/.relo`)이 유일한 환경 변수입니다. 설정은 `config.toml`에 있습니다.
[`config.example.toml`](config.example.toml)을 참고하세요.

비밀은 `$RELO_HOME/secrets.enc`에 있고 `$RELO_HOME/secret.key`로 봉인됩니다. `[secrets] keychain = true`이면 OS
키체인을 대신 사용합니다.

## ▓ 개발

```console
$ make build          # 콘솔 + daemon → ./relo
$ make seed           # 없으면 models.dev 빌드 시드를 내려받습니다
$ make dev-setup      # .dev/relo를 쓰고 ~/.relo는 건드리지 않습니다
$ make ci
```

Go 1.26+, Node 22+. CGO 없음. 나머지는 [CONTRIBUTING.md](CONTRIBUTING.md)와 [docs/](docs/README.md)를 보세요.
취약점 신고는 [SECURITY.md](SECURITY.md), 함께 배포되는 소프트웨어의 라이선스는
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)에 있습니다.

## ▓ 라이선스

Relo는 [GNU Affero General Public License v3.0](LICENSE)으로 배포됩니다. 함께 배포되는 서드파티 소프트웨어는 각자의
라이선스를 유지합니다. [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)를 참고하세요.
