<div align="center">
<img src="assets/logo.svg" alt="Relo" width="96">

<h2>Relo</h2>

<code>▚▚▚▚▚▚▚ Nhẹ • Nhanh • Ổn định ▚▚▚▚▚▚▚</code>

<a href="https://github.com/jonaskahn/relo/actions/workflows/ci.yml"><img src="https://github.com/jonaskahn/relo/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://codecov.io/gh/jonaskahn/relo"><img src="https://codecov.io/gh/jonaskahn/relo/branch/main/graph/badge.svg" alt="Coverage"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo"><img src="https://img.shields.io/npm/v/@jonaskahn/relo?label=npm%40latest" alt="npm latest"></a>
<a href="https://www.npmjs.com/package/@jonaskahn/relo?activeTab=versions"><img src="https://img.shields.io/npm/v/@jonaskahn/relo/beta" alt="npm beta"></a>

<p><i>Để các agent harness của bạn trò chuyện với mọi mô hình, mọi nhà cung cấp mà không cần thay đổi cấu hình.</i></p>

<p><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <a href="README.zh-Hans.md">简体中文</a> · <a href="README.de.md">Deutsch</a> · <a href="README.hi.md">हिन्दी</a> · <a href="README.ko.md">한국어</a> · <a href="README.ja.md">日本語</a></p>

</div>

---

## ▓ GIAO DIỆN ĐIỀU KHIỂN

<div align="center">

https://github.com/user-attachments/assets/cbda11c8-b60c-413b-8a08-03144dd06e7a

<sub>Bản demo dùng Relo</sub>
</div>

| Bảng điều khiển                                                                                                               | Kết nối                                                                                                               | Agent Harness                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| <img src="https://github.com/user-attachments/assets/664efc9a-cba4-4621-a63b-6aa6972ca751" alt="Bảng điều khiển" width="320"> | <img src="https://github.com/user-attachments/assets/66da3c81-3657-4d5e-867f-f670fa76187d" alt="Kết nối" width="320"> | <img src="https://github.com/user-attachments/assets/87ef4d68-c7b0-4847-afb3-f35f47e45685" alt="Nhóm" width="320"> |

## ▓ CÀI ĐẶT

Chọn một cách. Mọi trình cài đặt đều từ chối chồng thêm một Relo thứ hai.

- Linux & macOS

```shell
curl -fsSL https://relo.ifelse.one/install.sh | sh
```

- Windows

```shell
irm https://relo.ifelse.one/install.ps1 | iex
```

| Kênh           | Lệnh                                                                   |
| -------------- | ---------------------------------------------------------------------- |
| Script         | hai dòng ở trên; `install.sh --help` liệt kê các cờ                    |
| Homebrew       | `brew install jonaskahn/apps/relo`                                     |
| npm            | `npm i -g @jonaskahn/relo[@beta] --foreground-scripts` (Node 22+)      |
| Ứng dụng macOS | `Relo-<version>-universal.zip`                                         |
| Linux          | `Relo-Setup-<version>-amd64.deb` hoặc `Relo-Setup-<version>-amd64.rpm` |
| Windows        | `Relo-Setup-<version>-amd64.exe`                                       |

Các tệp cài đặt đến từ [bản phát hành mới nhất](https://github.com/jonaskahn/relo/releases/latest); sau đó ứng dụng
tự cập nhật. Script cài binary daemon: xác minh bản tải bằng `checksums.txt`, giải phóng các cổng Relo phục vụ và khởi
động daemon. Bản cài npm tải cùng binary 13 MB và in từng bước, kèm thanh tiến trình hiển thị tốc độ và thời gian còn
lại; `--foreground-scripts` giữ đầu ra đó ngay dưới lệnh cài thay vì ẩn đi.

Trên Windows, trình cài đặt setup và bản cài npm thêm lối tắt "Relo" trên Desktop, chạy `relo daemon restart --force`
mà không mở terminal.

## ▓ KHỞI CHẠY

```shell
relo daemon start                 # console tại http://127.0.0.1:10101
```

`daemon start` chạy proxy trong nền và hiện biểu tượng tray khi có phiên desktop. `daemon run` giữ nguyên tiến trình ở
foreground, đúng với cách terminal, mục đăng nhập và Relo.app dùng.

`daemon run|start|restart --silent` không bao giờ mở dashboard trong trình duyệt, đây là cách mục đăng nhập khởi động.

Chỉ dùng loopback. `[admin] login = true` đặt màn hình đăng nhập phía trước, dùng `$RELO_HOME/admin-token`.

## ▓ CỔNG VÀ TIỀN TỐ

| Cổng    | Giao thức                           |
| ------- | ----------------------------------- |
| `10101` | console + API quản trị + data plane |

| Tiền tố             | Giao thức                    |
| ------------------- | ---------------------------- |
| `/inference`        | Tương thích OpenAI (`/v1/*`) |
| `/inference-claude` | Anthropic (`/v1/*`)          |

Khóa client đi vào data plane. Admin token đi vào console. Thông tin xác thực nhà cung cấp chỉ đi lên upstream.

## ▓ LỆNH

```console
$ relo daemon run|start|stop|restart|status
$ relo autostart enable|disable
$ relo uninstall [--keep-data|--wipe]
$ relo version
```

`--home` và `--lang` ghi đè cho một lần chạy. Mọi thứ còn lại (kết nối, khóa, route) là một trang trong console.

`daemon stop --force` dừng daemon và giải phóng các cổng Relo phục vụ, đây là lệnh trình cài đặt chạy trước khi thay
binary. Cổng do chương trình khác giữ sẽ được báo, không bị kết thúc.

## ▓ GỠ CÀI ĐẶT

```console
$ relo uninstall            # hỏi dữ liệu có bị xóa không, rồi gỡ chương trình
$ relo uninstall --keep-data # dừng Relo, hủy đăng ký mục khởi động, giữ dữ liệu
$ relo uninstall --wipe      # như trên, và xóa $RELO_HOME cùng các mục keychain của nó
```

`relo uninstall` dừng daemon đang chạy, giải phóng mọi cổng đã cấu hình, tắt khởi động cùng hệ thống, xử lý dữ liệu
theo lựa chọn, nhận diện cách Relo đã được cài và gỡ theo đúng cách đó: trình gỡ của Windows setup, `npm uninstall -g`,
`brew uninstall`, xóa `Relo.app`, hoặc xóa binary do script cài. Bản cài `.deb` hoặc `.rpm` in lệnh đặc quyền thay vì
chạy lệnh đó. Có thể dùng cùng cách nhận diện khi binary không còn chạy được:

```shell
curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
```

```shell
irm https://relo.ifelse.one/uninstall.ps1 | iex
```

Mỗi cách gỡ giữ dữ liệu như sau:

| Cách                               | Dữ liệu                                |
| ---------------------------------- | -------------------------------------- |
| `relo uninstall`                   | hỏi; `y` xóa, mọi câu trả lời khác giữ |
| Windows: Settings → Apps           | hỏi trong hộp thoại; No giữ            |
| `apt remove relo`                  | giữ (`apt purge relo` xóa)             |
| `rpm -e relo`                      | giữ                                    |
| `npm uninstall -g @jonaskahn/relo` | giữ                                    |
| `brew uninstall relo`              | giữ                                    |

Cập nhật hoặc cài lại luôn giữ dữ liệu và dừng dịch vụ đang chạy trước, nên cài đè lên bản đang hoạt động không làm mất
gì.

## ▓ CẤU HÌNH

`$RELO_HOME` (`~/.relo`) là biến môi trường duy nhất. Cài đặt nằm trong `config.toml`. Xem
[`config.example.toml`](config.example.toml).

Bí mật nằm trong `$RELO_HOME/secrets.enc`, được niêm phong bằng `$RELO_HOME/secret.key`. `[secrets] keychain = true`
dùng keychain của hệ điều hành thay thế.

## ▓ PHÁT TRIỂN

```console
$ make build          # console + daemon → ./relo
$ make seed           # tải bản models.dev dùng để build khi chưa có
$ make dev-setup      # ghi .dev/relo, không đụng tới ~/.relo
$ make ci
```

Go 1.26+, Node 22+. Không CGO. Xem [CONTRIBUTING.md](CONTRIBUTING.md) và [docs/](docs/README.md) để biết phần còn
lại, [SECURITY.md](SECURITY.md) để báo lỗ hổng, và [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) cho giấy phép
của phần mềm đi kèm.

## ▓ GIẤY PHÉP

Relo được cấp phép theo [GNU Affero General Public License v3.0](LICENSE). Phần mềm bên thứ ba đi kèm giữ giấy phép
riêng; xem [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
