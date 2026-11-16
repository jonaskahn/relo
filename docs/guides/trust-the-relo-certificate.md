# Trust the Relo certificate yourself

Relo's **Route Cowork** switch (Claude Desktop → Desktop picker) needs your computer to trust one
certificate: **Relo Local CA**. Relo normally asks your system for that itself and shows the system's own
approval dialog. This page is for when it cannot, when you turned the dialog down, or when you would
rather do it by hand. It also shows how to check the certificate and how to remove it.

## What the certificate is

- It is created on your machine the first time you use proxy mode. It lives in `~/.relo/ca/ca.pem`
  (on Windows, `%USERPROFILE%\.relo\ca\ca.pem`), with its private key `ca.key` beside it.
- It is **limited to two names**, `claude.ai` and `api.anthropic.com`, and excludes every IP address. A
  system that honours name constraints, which macOS, Windows and Linux do, will not accept it for any
  other site.
- Trusting it system-wide makes **every app and every user** on this computer accept a certificate Relo
  issues for those two names. Anything that runs as you can read `ca.key`. Install it only if you accept
  that. Anthropic's terms may treat intercepting its traffic as a violation, and an account can be
  suspended.
- Relo removes it for you when Claude Desktop leaves proxy mode, is removed, or Relo is uninstalled. Turning
  Route Cowork off alone leaves it in place until you remove it.

## Check that the file is the right one

Before you trust anything, confirm the file's SHA-256 fingerprint matches the one Relo shows in the
console (it is the same value in the "Do it yourself" panel).

```sh
# macOS and Linux
openssl x509 -in ~/.relo/ca/ca.pem -noout -fingerprint -sha256
```

```powershell
# Windows
certutil -hashfile "$env:USERPROFILE\.relo\ca\ca.pem" SHA256
```

The value must equal the fingerprint in the console, ignoring colons and letter case. If it does not,
stop and do not trust the file.

## macOS

Install. Run it as yourself, not under `sudo`: macOS asks you for your administrator password in its own
dialog, and a command run as root has no screen to ask on and is refused.

```sh
security add-trusted-cert -d -r trustRoot -p ssl ~/.relo/ca/ca.pem
```

Remove (the second line only matters if the certificate was also copied into the System keychain):

```sh
security remove-trusted-cert -d ~/.relo/ca/ca.pem
sudo security delete-certificate -Z <SHA-1 fingerprint> /Library/Keychains/System.keychain
```

Check:

```sh
security dump-trust-settings -d | grep "Relo Local CA"
```

You can also use Keychain Access: drag the file into **System**, open it, expand **Trust** and set
**Secure Sockets Layer (SSL)** to **Always Trust**. Apple's guide:
<https://support.apple.com/guide/keychain-access/welcome/mac>.

## Windows

Run these in a Command Prompt opened **as administrator**.

Install:

```bat
certutil -addstore -f Root "%USERPROFILE%\.relo\ca\ca.pem"
```

Remove:

```bat
certutil -delstore Root <SHA-1 fingerprint>
```

Check:

```bat
certutil -store Root <SHA-1 fingerprint>
```

Microsoft's documentation: <https://learn.microsoft.com/windows-server/administration/windows-commands/certutil>.

## Linux

Use the line for your distribution.

| Distribution | Install | Remove |
|---|---|---|
| Debian, Ubuntu | `sudo cp ~/.relo/ca/ca.pem /usr/local/share/ca-certificates/relo-local-ca.crt && sudo update-ca-certificates` | `sudo rm /usr/local/share/ca-certificates/relo-local-ca.crt && sudo update-ca-certificates --fresh` |
| Fedora, RHEL | `sudo cp ~/.relo/ca/ca.pem /etc/pki/ca-trust/source/anchors/relo-local-ca.crt && sudo update-ca-trust` | `sudo rm /etc/pki/ca-trust/source/anchors/relo-local-ca.crt && sudo update-ca-trust` |
| Arch (p11-kit) | `sudo trust anchor --store ~/.relo/ca/ca.pem` | `sudo trust anchor --remove ~/.relo/ca/ca.pem` |

Chromium-based apps, which includes Electron apps such as Claude Desktop, read your user NSS database
as well. If `certutil` (package `libnss3-tools`) is installed:

```sh
certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n "Relo Local CA" -i ~/.relo/ca/ca.pem
certutil -d sql:$HOME/.pki/nssdb -D -n "Relo Local CA"   # to remove it
```

## Let your coding agent do it

The console's **Copy a prompt for your coding agent** button copies a prompt that already contains the
file path, the fingerprint to verify and the commands for your system. It tells the agent to verify the
fingerprint first, trust only that file, ask you before running anything that needs administrator
rights, and report what it saw. Paste it into the agent you use. Read the commands before you approve them.

## After you install it by hand

Press **Check again** in the console. Relo reads the system's trust without changing anything, and once
it sees the certificate it lets you turn Route Cowork on. Then quit Claude Desktop completely and start a
new Cowork session, because Cowork's virtual machine copies the system's trust when it starts.

## Remove it

Use the **Remove system-wide trust** button in the console, or the removal commands above. After removing
it, `dump-trust-settings -d` (macOS), `certutil -store Root` (Windows) or the Linux check should no
longer list **Relo Local CA**.
