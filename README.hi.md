<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ हल्का • तेज़ • भरोसेमंद ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>अपने harness agents को किसी भी मॉडल और किसी भी प्रदाता से जोड़ें — बिना कॉन्फ़िगरेशन बदले।</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ कंसोल

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Relo का डेमो</sub>
</div>

| डैशबोर्ड                                                                                                               | कनेक्शन                                                                                                               | Agent Harness                                                                                                      |
| ---------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="डैशबोर्ड" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="कनेक्शन" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="समूह" width="320"> |

## ▓ इंस्टॉल

कोई एक तरीका चुनें। हर इंस्टॉलर दूसरा Relo ऊपर लगाने से इनकार करता है।

- Linux और macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| चैनल      | कमांड                                                                |
| --------- | -------------------------------------------------------------------- |
| स्क्रिप्ट | ऊपर की दो पंक्तियाँ; `install.sh --help` सभी विकल्प बताता है         |
| Homebrew  | `brew install jonaskahn/apps/relo`                                   |
| npm       | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts` (Node 22+)    |
| macOS ऐप  | `Relo-<version>-universal.zip`                                       |
| Linux     | `Relo-Setup-<version>-amd64.deb` या `Relo-Setup-<version>-amd64.rpm` |
| Windows   | `Relo-Setup-<version>-amd64.exe`                                     |

सेटअप फ़ाइलें [नवीनतम रिलीज़](https://github.com/jonaskahn/relo/releases/latest) से आती हैं; इसके बाद ऐप खुद को अपडेट
करता है। स्क्रिप्ट daemon बाइनरी इंस्टॉल करती है: यह `checksums.txt` से डाउनलोड जाँचती है, Relo के इस्तेमाल वाले पोर्ट
खाली करती है और daemon शुरू करती है। npm इंस्टॉल वही 13 MB बाइनरी डाउनलोड करता है और हर चरण प्रिंट करता है, साथ में
गति और बचा समय दिखाने वाला प्रोग्रेस बार; `--foreground-scripts` उस आउटपुट को छिपाने के बजाय इंस्टॉल कमांड के नीचे
रखता है।

Windows पर सेटअप इंस्टॉलर और npm इंस्टॉल डेस्कटॉप पर "Relo" शॉर्टकट जोड़ते हैं, जो बिना टर्मिनल खोले
`relo daemon restart --force` चलाता है।

## ▓ शुरुआत

```shell
relo daemon start                 # कंसोल http://127.0.0.1:10101 पर
```

`daemon start` प्रॉक्सी को बैकग्राउंड में चलाता है और डेस्कटॉप सेशन उपलब्ध होने पर tray आइकन दिखाता है। `daemon run`
वही प्रोसेस foreground में रखता है — टर्मिनल, लॉगिन आइटम और Relo.app इसी तरह चलते हैं।

`daemon run|start|restart --silent` कभी भी ब्राउज़र में डैशबोर्ड नहीं खोलता; लॉगिन आइटम इसी तरह शुरू होता है।

सिर्फ़ loopback। `[admin] login = true` आगे साइन-इन लगाता है, जो `$RELO_HOME/admin-token` इस्तेमाल करता है।

## ▓ पोर्ट और प्रीफ़िक्स

| पोर्ट   | क्या बोलता है                    |
| ------- | -------------------------------- |
| `10101` | कंसोल + प्रबंधन API + data plane |

| प्रीफ़िक्स          | क्या बोलता है         |
| ------------------- | --------------------- |
| `/inference`        | OpenAI-संगत (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)   |

क्लाइंट कुंजियाँ data plane पर जाती हैं, admin token कंसोल पर। प्रदाता की साख सिर्फ़ upstream जाती है।

## ▓ कमांड

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` और `--lang` एक बार के लिए सेटिंग बदलते हैं। बाकी सब कुछ (कनेक्शन, कुंजियाँ, रूट) कंसोल के पेज हैं।

`daemon stop --force` daemon रोकता है और Relo के इस्तेमाल वाले पोर्ट खाली करता है; इंस्टॉलर बाइनरी बदलने से पहले यही
चलाता है। किसी दूसरे प्रोग्राम का पोर्ट बताया जाता है, बंद नहीं किया जाता।

## ▓ अनइंस्टॉल

```console
$ relo uninstall            # डेटा के बारे में पूछता है, फिर प्रोग्राम हटाता है
$ relo uninstall --keep-data # Relo रोकता है, लॉगिन आइटम हटाता है, डेटा रखता है
$ relo uninstall --wipe      # वही, और $RELO_HOME तथा उसके keychain entries भी मिटाता है
```

`relo uninstall` चल रहा daemon रोकता है, हर कॉन्फ़िगर किया गया पोर्ट खाली करता है, लॉगिन पर शुरू होना बंद करता है, डेटा
का फ़ैसला करता है, Relo कैसे इंस्टॉल हुआ यह पहचानता है और उसी तरह हटाता है: Windows सेटअप का अनइंस्टॉलर,
`npm uninstall -g`, `brew uninstall`, `Relo.app` हटाना, या स्क्रिप्ट से इंस्टॉल बाइनरी हटाना। `.deb` या `.rpm`
इंस्टॉल प्रिविलेज वाला कमांड चलाने के बजाय प्रिंट करता है। बाइनरी चल न पाने पर भी वही पहचान उपलब्ध है:

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

हर तरीका डेटा कैसे रखता है:

| तरीका                              | डेटा                                  |
| ---------------------------------- | ------------------------------------- |
| `relo uninstall`                   | पूछता है; `y` मिटाता है, बाकी रखता है |
| Windows: Settings → Apps           | डायलॉग में पूछता है; No रखता है       |
| `apt remove relo`                  | रखता है (`apt purge relo` मिटाता है)  |
| `rpm -e relo`                      | रखता है                               |
| `npm uninstall -g @jonaskahn/relo` | रखता है                               |
| `brew uninstall relo`              | रखता है                               |

अपडेट या रीइंस्टॉल हमेशा डेटा रखता है और पहले चल रही सेवा रोकता है, इसलिए चालू इंस्टॉल के ऊपर इंस्टॉल करने से कुछ नहीं
जाता।

## ▓ कॉन्फ़िगरेशन

`$RELO_HOME` (`~/.relo`) एकमात्र env var है। सेटिंग्स `config.toml` में रहती हैं। देखें
[`config.example.toml`](config.example.toml)।

सीक्रेट `$RELO_HOME/secrets.enc` में रहते हैं, जिन्हें `$RELO_HOME/secret.key` सील करती है।
`[secrets] keychain = true` इसके बजाय OS keychain इस्तेमाल करता है।

## ▓ डेवलपमेंट

```console
$ make build          # कंसोल + daemon → ./relo
$ make seed           # मौजूद न हो तो models.dev build seed डाउनलोड करें
$ make dev-setup      # .dev/relo लिखता है, ~/.relo को नहीं छूता
$ make ci
```

Go 1.26+, Node 22+। CGO नहीं। बाकी के लिए [CONTRIBUTING.md](CONTRIBUTING.md) और [docs/](docs/README.md) देखें,
कमज़ोरी रिपोर्ट करने के लिए [SECURITY.md](SECURITY.md), और साथ भेजे गए सॉफ़्टवेयर के लाइसेंस के लिए
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)।

## ▓ लाइसेंस

Relo [GNU Affero General Public License v3.0](LICENSE) के अंतर्गत लाइसेंस्ड है। साथ भेजा गया तीसरे पक्ष का सॉफ़्टवेयर
अपने लाइसेंस रखता है; देखें [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)।
