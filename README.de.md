<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Leicht • Schnell • Zuverlässig ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>Lass deine Harness-Agents mit jedem Modell und jedem Anbieter sprechen – ohne Änderungen an der Konfiguration.</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ KONSOLE

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Demo mit Relo</sub>
</div>

| Dashboard                                                                                                               | Verbindungen                                                                                                               | Agent Harness                                                                                                         |
| ----------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="Dashboard" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="Verbindungen" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="Gruppen" width="320"> |

## ▓ INSTALLATION

Einen Weg wählen. Jeder Installer weigert sich, ein zweites Relo daneben zu legen.

- Linux & macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| Kanal     | Befehl                                                                 |
| --------- | ---------------------------------------------------------------------- |
| Skript    | die zwei Zeilen oben; `install.sh --help` listet die Optionen          |
| Homebrew  | `brew install jonaskahn/apps/relo`                                     |
| npm       | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts` (Node 22+)      |
| macOS-App | `Relo-<version>-universal.zip`                                         |
| Linux     | `Relo-Setup-<version>-amd64.deb` oder `Relo-Setup-<version>-amd64.rpm` |
| Windows   | `Relo-Setup-<version>-amd64.exe`                                       |

Die Setup-Dateien stammen aus dem [neuesten Release](https://github.com/jonaskahn/relo/releases/latest); danach aktualisiert
sich die App selbst. Das Skript installiert das Daemon-Binary: Es prüft den Download gegen `checksums.txt`, gibt die von
Relo belegten Ports frei und startet den Daemon. Die npm-Installation lädt dasselbe 13-MB-Binary und zeigt jeden Schritt
mit einem Fortschrittsbalken samt Rate und Restzeit; `--foreground-scripts` lässt diese Ausgabe unter dem Installationsbefehl
stehen, statt sie zu verbergen.

Unter Windows legen das Setup und die npm-Installation eine Desktop-Verknüpfung „Relo“ an, die
`relo daemon restart --force` ohne Terminalfenster ausführt.

## ▓ START

```shell
relo daemon start                 # Konsole unter http://127.0.0.1:10101
```

`daemon start` lässt den Proxy im Hintergrund laufen und zeigt das Tray-Symbol, wenn eine Desktop-Sitzung verfügbar ist.
`daemon run` hält denselben Prozess im Vordergrund – so starten ein Terminal, ein Login-Element und Relo.app.

`daemon run|start|restart --silent` öffnet das Dashboard nie im Browser; genau so startet ein Login-Element.

Nur Loopback. `[admin] login = true` setzt eine Anmeldung davor, die `$RELO_HOME/admin-token` verwendet.

## ▓ ENDPUNKTE

| Port    | Spricht                               |
| ------- | ------------------------------------- |
| `10101` | Konsole + Management-API + Data Plane |

| Präfix              | Spricht                     |
| ------------------- | --------------------------- |
| `/inference`        | OpenAI-kompatibel (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)         |

Client-Schlüssel gehen an die Data Plane, das Admin-Token an die Konsole. Anbieter-Zugangsdaten gehen nur upstream.

## ▓ BEFEHLE

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` und `--lang` überschreiben eine Einstellung für einen Lauf. Alles andere (Verbindungen, Schlüssel, Routen) ist
eine Seite in der Konsole.

`daemon stop --force` stoppt den Daemon und gibt die von Relo belegten Ports frei; genau das führt ein Installer aus, bevor
er die Binaries ersetzt. Ein Port, den ein anderes Programm hält, wird gemeldet statt beendet.

## ▓ DEINSTALLATION

```console
$ relo uninstall            # fragt nach den Daten und entfernt dann das Programm
$ relo uninstall --keep-data # stoppt Relo, entfernt Login-Elemente, behält die Daten
$ relo uninstall --wipe      # dasselbe, löscht aber $RELO_HOME und dessen Keychain-Einträge
```

`relo uninstall` stoppt den laufenden Daemon, gibt alle konfigurierten Ports frei, schaltet den Start bei der Anmeldung aus,
klärt die Daten, erkennt die Installationsart und entfernt Relo auf demselben Weg: Deinstaller des Windows-Setups,
`npm uninstall -g`, `brew uninstall`, Löschen von `Relo.app` oder Löschen des per Skript installierten Binaries. Bei einer
`.deb`- oder `.rpm`-Installation wird der privilegierte Befehl ausgegeben statt ausgeführt. Dieselbe Erkennung gibt es auch
ohne lauffähiges Binary:

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

Was die einzelnen Wege behalten:

| Weg                                | Daten                                  |
| ---------------------------------- | -------------------------------------- |
| `relo uninstall`                   | fragt; `y` löscht, alles andere behält |
| Windows: Einstellungen → Apps      | fragt im Dialog; „Nein“ behält         |
| `apt remove relo`                  | behält (`apt purge relo` löscht)       |
| `rpm -e relo`                      | behält                                 |
| `npm uninstall -g @jonaskahn/relo` | behält                                 |
| `brew uninstall relo`              | behält                                 |

Ein Update oder eine Neuinstallation behält die Daten immer und stoppt zuerst den laufenden Dienst; eine Installation über
eine funktionierende hinweg verliert also nichts.

## ▓ KONFIGURATION

`$RELO_HOME` (`~/.relo`) ist die einzige Umgebungsvariable. Die Einstellungen stehen in `config.toml`. Siehe
[`config.example.toml`](config.example.toml).

Geheimnisse liegen in `$RELO_HOME/secrets.enc`, versiegelt durch `$RELO_HOME/secret.key`. Mit
`[secrets] keychain = true` wird stattdessen der Keychain des Betriebssystems verwendet.

## ▓ ENTWICKLUNG

```console
$ make build          # Konsole + Daemon → ./relo
$ make seed           # lädt den models.dev-Build-Seed, wenn er fehlt
$ make dev-setup      # schreibt .dev/relo und lässt ~/.relo in Ruhe
$ make ci
```

Go 1.26+, Node 22+. Kein CGO. Der Rest steht in [CONTRIBUTING.md](CONTRIBUTING.md) und [docs/](docs/README.md);
Sicherheitslücken bitte über [SECURITY.md](SECURITY.md) melden, Lizenzen gebündelter Software in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## ▓ LIZENZ

Relo steht unter der [GNU Affero General Public License v3.0](LICENSE). Gebündelte Drittanbieter-Software behält ihre
eigenen Lizenzen; siehe [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
