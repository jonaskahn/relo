"""mitmproxy addon: save full request and response headers and body to JSONL files.

One line per request, written when the request arrives so hung or
interrupted exchanges are still on disk, plus one line per response
when it arrives.

Deliberate simplification: whole bodies are held in memory. Fine for
debugging traffic; a streaming writer is the upgrade path if multi-GB
uploads ever flow through here.
"""

import base64
import json
import os
import re
import time
from datetime import datetime, timezone

CAPTURE_DIR = os.environ.get("CAPTURE_DIR", "debugger/captures")
SHOW_SECRETS = os.environ.get("CAPTURE_SHOW_SECRETS", "") == "1"
RUN_ID = os.environ.get("CAPTURE_RUN_ID", "manual")

# Mirrors daemon/internal/adapters/upstream/redact.go plus the console copy.
_SENSITIVE_NAMES = {
    "authorization",
    "proxyauthorization",
    "apikey",
    "xapikey",
    "xgoogapikey",
    "xamzsecuritytoken",
    "xapitoken",
    "xauthtoken",
    "xxaitokenauth",
    "authentication",
    "password",
    "cookie",
    "setcookie",
}
_SENSITIVE_SUFFIXES = ("apikey", "secret", "password", "token")
_MASK = "\u2022\u2022\u2022\u2022"
_SCHEME = re.compile(r"^[A-Za-z]{2,12}\s+\S")
_CREDENTIAL_FIELD = re.compile(
    r'(?i)"(api[-_]?key|apikeys?|access[-_]?token|auth[-_]?token|'
    r"refresh[-_]?token|id[-_]?token|token|secret|client[-_]?secret|"
    r'password|authorization|credentials?)"(\s*:\s*)"[^"]*"'
)


def _sensitive_header(name):
    normalized = re.sub(r"[-_\s]", "", name.lower())
    if normalized in _SENSITIVE_NAMES:
        return True
    return normalized.endswith(_SENSITIVE_SUFFIXES)


def _mask_value(value):
    trimmed = value.strip()
    if not trimmed:
        return value
    if _SCHEME.match(trimmed):
        return trimmed.split(None, 1)[0] + " " + _MASK
    return _MASK


def _mask_headers(headers):
    out = {}
    for name, values in headers.items():
        if SHOW_SECRETS or not _sensitive_header(name):
            out[name] = list(values)
        else:
            out[name] = [_mask_value(v) for v in values]
    return out


def _mask_body_text(text):
    if SHOW_SECRETS:
        return text
    return _CREDENTIAL_FIELD.sub(r"\1\2" + '"%s"' % _MASK, text)


def _encode_body(raw):
    try:
        return "text", _mask_body_text(raw.decode("utf-8"))
    except UnicodeDecodeError:
        return "base64", base64.b64encode(raw).decode("ascii")


def _headers_dict(headers):
    out = {}
    for name, value in headers.items(multi=True):
        out.setdefault(name, []).append(value)
    return out


_file = None


def _stream():
    global _file
    if _file is None:
        os.makedirs(CAPTURE_DIR, mode=0o700, exist_ok=True)
        stamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
        path = os.path.join(CAPTURE_DIR, "capture-%s-%s.jsonl" % (stamp, RUN_ID))
        _file = open(path, "a", encoding="utf-8")
        os.chmod(path, 0o600)
        _file.write(
            json.dumps({"type": "run", "run_id": RUN_ID, "started": stamp}) + "\n"
        )
        _file.flush()
    return _file


def request(flow):
    req = flow.request
    raw = bytes(req.content or b"")
    encoding, body = _encode_body(raw)
    record = {
        "type": "request",
        "ts": time.time(),
        "run_id": RUN_ID,
        "method": req.method,
        "url": req.pretty_url,
        "http_version": req.http_version,
        "headers": _mask_headers(_headers_dict(req.headers)),
        "masked": not SHOW_SECRETS,
        "body_encoding": encoding,
        "body": body,
    }
    stream = _stream()
    stream.write(json.dumps(record) + "\n")
    stream.flush()


def response(flow):
    req = flow.request
    resp = flow.response
    if resp is None:
        return
    raw = bytes(resp.content or b"")
    encoding, body = _encode_body(raw)
    record = {
        "type": "response",
        "ts": time.time(),
        "run_id": RUN_ID,
        "method": req.method,
        "url": req.pretty_url,
        "status_code": resp.status_code,
        "reason": getattr(resp, "reason", ""),
        "http_version": resp.http_version,
        "headers": _mask_headers(_headers_dict(resp.headers)),
        "masked": not SHOW_SECRETS,
        "body_encoding": encoding,
        "body": body,
    }
    stream = _stream()
    stream.write(json.dumps(record) + "\n")
    stream.flush()
