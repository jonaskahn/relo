#!/usr/bin/env node
// Compare one genuine client request with what Relo sent upstream.
//
// Usage:
//   node scripts/compare-capture.mjs --reference <capture.jsonl> --event 59995
//   node scripts/compare-capture.mjs --reference <capture.jsonl> --event 59995 \
//     [--base-url http://127.0.0.1:10101] [--admin-token <token>]
//     [--admin-token-file <path>] [--relo-sqlite <state.sqlite>]
//     [--show-secrets] [--ignore-header <name>] [--ignore-key <key>]
//
// The reference is a mitmproxy JSONL from scripts/capture.mjs. Relo's side is
// read from the management API, falling back to a read-only sqlite3 query.
// Exit 0 when the two normalize to the same request, 1 when they differ.
// Sensitive header values are never printed unless --show-secrets is passed.
import {execFileSync} from "node:child_process";
import {readFileSync} from "node:fs";
import {homedir} from "node:os";
import {join} from "node:path";

const DEFAULT_BASE_URL = "http://127.0.0.1:10101";
const DEFAULT_TOKEN_FILE = join(homedir(), ".relo", "admin-token");

// Transport and correlation headers say nothing about the request Relo built,
// and every hop rewrites them: the diff leaves them out.
const IGNORED_HEADERS = new Set([
    "connection", "content-length", "host", "accept-encoding", "b3",
    "traceparent", "tracestate", "date", "server", "cf-ray", "cf-placement",
    "x-opencode-log-id", "server-timing", "retry-after", "transfer-encoding",
    // Per-attempt correlation ids: the value is minted for one send and says
    // nothing about the shape Relo builds.
    "x-opencode-request",
]);

const SENSITIVE_HEADERS = new Set([
    "authorization", "proxy-authorization", "cookie", "set-cookie",
    "x-api-key", "api-key", "x-goog-api-key", "x-auth-token", "authentication",
]);

// Per-conversation identifiers differ by design between the two captures, so
// the diff compares their presence and shape, not their value.
const IDENTITY_HEADERS = new Set([
    "x-opencode-session", "x-opencode-session-id", "x-session-affinity",
    "x-session-id", "x-opencode-parent-session-id", "x-parent-session-id",
    "x-opencode-project",
]);

const fail = (message) => {
    console.error(`compare-capture: ${message}`);
    process.exit(2);
};

const args = process.argv.slice(2);

function opt(name, fallback) {
    const index = args.indexOf(name);
    return index >= 0 && index + 1 < args.length ? args[index + 1] : fallback;
}

function optAll(name) {
    const values = [];
    for (let index = 0; index < args.length; index++) {
        if (args[index] === name && index + 1 < args.length) values.push(args[index + 1]);
    }
    return values;
}

function flag(name) {
    return args.includes(name);
}

const reference = opt("--reference");
const eventID = opt("--event");
if (!reference || !eventID) fail("usage: node scripts/compare-capture.mjs --reference <capture.jsonl> --event <id>");

const baseURL = opt("--base-url", DEFAULT_BASE_URL).replace(/\/+$/, "");
const showSecrets = flag("--show-secrets");
const ignoredHeaders = new Set([...IGNORED_HEADERS, ...optAll("--ignore-header").map((name) => name.toLowerCase())]);
const ignoredKeys = new Set(optAll("--ignore-key"));

function readJSONL(path) {
    return readFileSync(path, "utf8")
        .split("\n")
        .filter((line) => line.trim() !== "")
        .map((line) => {
            try {
                return JSON.parse(line);
            } catch {
                return null;
            }
        })
        .filter(Boolean);
}

const records = readJSONL(reference);
const requests = records.filter((record) => record.type === "request");
if (requests.length === 0) fail(`no request records in ${reference}`);

function bodyOf(record) {
    if (typeof record.body !== "string") return null;
    if (record.body_encoding === "base64") return null;
    try {
        return JSON.parse(record.body);
    } catch {
        return null;
    }
}

function headersOf(record) {
    const out = new Map();
    for (const [name, values] of Object.entries(record.headers ?? {})) {
        const key = name.toLowerCase();
        if (ignoredHeaders.has(key)) continue;
        out.set(key, Array.isArray(values) ? values.join(", ") : String(values));
    }
    return out;
}

// normalizePath keeps the address that was asked for while folding away ids
// that differ per request by design.
function normalizePath(raw) {
    let url;
    try {
        url = new URL(raw);
    } catch {
        return raw;
    }
    const path = url.pathname
        .replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, ":id")
        .replace(/[0-9a-f]{24,}/gi, ":id")
        .replace(/\/+$/, "");
    return `${url.host.toLowerCase()}${path}`;
}

function maskedHeader(name, value) {
    if (!SENSITIVE_HEADERS.has(name)) return value;
    if (showSecrets) return value;
    const space = value.indexOf(" ");
    return space > 0 ? `${value.slice(0, space)} ****` : "****";
}

// normalizeHeader folds the values that legitimately vary between two runs:
// conversation ids, forged project ids, and the client build version Relo
// relays from whatever agent called it.
function normalizeHeader(name, value) {
    if (IDENTITY_HEADERS.has(name)) return "<present>";
    if (name === "user-agent") {
        return value.replace(/(opencode\/latest\/)[^/]+(\/)/i, "$1<version>$2");
    }
    return value;
}

function readAdminToken() {
    const direct = opt("--admin-token");
    if (direct) return direct;
    const file = opt("--admin-token-file", DEFAULT_TOKEN_FILE);
    try {
        return readFileSync(file, "utf8").trim();
    } catch {
        return "";
    }
}

async function readReloFromAPI() {
    const token = readAdminToken();
    if (!token) throw new Error("no admin token; pass --admin-token or --admin-token-file");
    const headers = {Authorization: `Bearer ${token}`};
    const listed = await fetch(`${baseURL}/api/v1/activity/requests/${eventID}/captures`, {headers});
    if (!listed.ok) throw new Error(`captures API answered ${listed.status}`);
    const payload = await listed.json();
    const items = payload.items ?? [];
    const capture = items.find((item) => item.kind === "provider_request") ??
        items.find((item) => item.kind === "agent_request");
    if (!capture) throw new Error(`event ${eventID} stores no request capture`);
    const body = await fetch(`${baseURL}/api/v1/activity/requests/${eventID}/captures/${capture.id}/body`, {headers});
    if (!body.ok) throw new Error(`capture body API answered ${body.status}`);
    return {capture, body: await body.text()};
}

function readReloFromSQLite(path) {
    const query = `SELECT id, kind, method, url, headers, cast(body as text) AS body
        FROM usage_captures WHERE event_id = ${Number(eventID)}
          AND kind IN ('provider_request', 'agent_request')
        ORDER BY CASE kind WHEN 'provider_request' THEN 0 ELSE 1 END, id LIMIT 1`;
    const raw = execFileSync("sqlite3", ["-readonly", "-json", path, query], {encoding: "utf8"});
    const rows = JSON.parse(raw || "[]");
    if (rows.length === 0) throw new Error(`event ${eventID} stores no request capture`);
    const row = rows[0];
    return {capture: {...row, headers: JSON.parse(row.headers || "{}")}, body: row.body ?? ""};
}

async function readRelo() {
    const sqlitePath = opt("--relo-sqlite");
    // The API is the primary source; the database is the documented fallback
    // for a daemon that is not running.
    const attempts = sqlitePath
        ? [readReloFromAPI, () => readReloFromSQLite(sqlitePath)]
        : [readReloFromAPI];
    let lastError;
    for (const attempt of attempts) {
        try {
            return await attempt();
        } catch (error) {
            lastError = error;
        }
    }
    throw lastError;
}

const relo = await readRelo();
const reloBody = (() => {
    try {
        return JSON.parse(relo.body);
    } catch {
        return null;
    }
})();

// A capture turns often share one model (a title turn and the main turn). The
// main turn is the largest chat request carrying that model.
const candidates = requests.filter((record) => (record.url ?? "").includes("/chat/completions"));
const pool = candidates.length > 0 ? candidates : requests;
const sameModel = reloBody?.model
    ? pool.filter((record) => bodyOf(record)?.model === reloBody.model)
    : pool;
const chosen = (sameModel.length > 0 ? sameModel : pool).reduce((largest, record) =>
    (record.body ?? "").length > (largest.body ?? "").length ? record : largest);

const referenceSide = {
    method: (chosen.method ?? "").toUpperCase(),
    path: normalizePath(chosen.url ?? ""),
    headers: headersOf(chosen),
    body: bodyOf(chosen),
};
const reloSide = {
    method: (relo.capture.method ?? "").toUpperCase(),
    path: normalizePath(relo.capture.url ?? ""),
    headers: headersOf(relo.capture),
    body: reloBody,
};

const differences = [];

function compareScalar(label, left, right) {
    if (left === right) return;
    differences.push(`${label}: reference=${JSON.stringify(left)} relo=${JSON.stringify(right)}`);
}

compareScalar("method", referenceSide.method, reloSide.method);
compareScalar("path", referenceSide.path, reloSide.path);

const headerNames = new Set([...referenceSide.headers.keys(), ...reloSide.headers.keys()]);
for (const name of [...headerNames].sort()) {
    const left = referenceSide.headers.get(name);
    const right = reloSide.headers.get(name);
    if (left === undefined) {
        differences.push(`header +${name}: ${maskedHeader(name, right)}`);
        continue;
    }
    if (right === undefined) {
        differences.push(`header -${name}: ${maskedHeader(name, left)}`);
        continue;
    }
    if (SENSITIVE_HEADERS.has(name) && !showSecrets) continue;
    const normalizedLeft = normalizeHeader(name, left);
    const normalizedRight = normalizeHeader(name, right);
    if (normalizedLeft !== normalizedRight) {
        differences.push(`header ~${name}: reference=${JSON.stringify(maskedHeader(name, left))} relo=${JSON.stringify(maskedHeader(name, right))}`);
    }
}

if (!referenceSide.body || !reloSide.body) {
    differences.push("body: one side did not carry a JSON object");
} else {
    // The output ceiling travels under one of two names on the Chat wire;
    // compare the value once instead of the spelling.
    const ceiling = (body) => body.max_completion_tokens ?? body.max_tokens;
    const keys = new Set([...Object.keys(referenceSide.body), ...Object.keys(reloSide.body)]);
    keys.delete("max_completion_tokens");
    keys.delete("max_tokens");
    keys.add("max_tokens");
    for (const key of [...keys].sort()) {
        if (ignoredKeys.has(key)) continue;
        const left = key === "max_tokens" ? ceiling(referenceSide.body) : referenceSide.body[key];
        const right = key === "max_tokens" ? ceiling(reloSide.body) : reloSide.body[key];
        if (left === undefined) {
            differences.push(`body +${key}`);
            continue;
        }
        if (right === undefined) {
            differences.push(`body -${key}`);
            continue;
        }
        // Arrays and objects are compared for presence only: their contents
        // are conversation data, not wire shape.
        if (left !== null && typeof left === "object") {
            if (key === "stream_options") {
                compareScalar(`body.${key}`, JSON.stringify(left), JSON.stringify(right));
            }
            continue;
        }
        compareScalar(`body.${key}`, left, right);
    }
}

console.log(`reference ${reference}`);
console.log(`relo      event ${eventID} (${reloSide.method} ${reloSide.path})`);
if (differences.length === 0) {
    console.log("no differences");
    process.exit(0);
}
for (const difference of differences) console.log(difference);
console.log(`${differences.length} difference${differences.length === 1 ? "" : "s"}`);
process.exit(1);
