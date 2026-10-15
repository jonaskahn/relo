#!/usr/bin/env node
// Capture agent HTTP traffic via mitmproxy, saving full headers and body to files.
//
// Usage:
//   node scripts/capture.mjs setup [--port 18080] [--trust]
//   node scripts/capture.mjs run [--port 18080] [--show-secrets] [--local name] -- <client> [args]
//   node scripts/capture.mjs teardown [--purge]
//
// Regular proxy mode sets HTTPS_PROXY/HTTP_PROXY plus CA env vars on the child
// only, with NO_PROXY cleared so localhost (Relo) is captured too. --local
// uses mitmproxy local-capture mode for clients that ignore proxy env vars.
// Secrets are masked by default; --show-secrets saves raw values for debugging.
import {spawn, spawnSync} from "node:child_process";
import {chmodSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync} from "node:fs";
import {homedir} from "node:os";
import {dirname, join, resolve} from "node:path";
import {connect} from "node:net";

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), "..");
const OUT_DIR = join(ROOT, "debugger", "captures");
const STATE_FILE = join(ROOT, "debugger", ".capture-state.json");
const PID_FILE = join(ROOT, "debugger", ".capture.pid");
const ADDON = join(ROOT, "scripts", "capture-addon.py");
const CA = join(homedir(), ".mitmproxy", "mitmproxy-ca-cert.pem");
const DEFAULT_PORT = 18080;

const fail = (msg) => {
    console.error(`capture: ${msg}`);
    process.exit(1);
};
const info = (msg) => console.log(`capture: ${msg}`);

function flag(args, ...names) {
    for (const n of names) if (args.includes(n)) return true;
    return false;
}

function opt(args, name, fallback) {
    const i = args.indexOf(name);
    return i >= 0 && i + 1 < args.length ? args[i + 1] : fallback;
}

function writeState(patch) {
    mkdirSync(dirname(STATE_FILE), {recursive: true, mode: 0o700});
    let cur = {};
    try {
        cur = JSON.parse(readFileSync(STATE_FILE, "utf8"));
    } catch {
    }
    writeFileSync(STATE_FILE, JSON.stringify({...cur, ...patch}, null, 2));
    chmodSync(STATE_FILE, 0o600);
}

function waitPort(port, ms = 10000) {
    const start = Date.now();
    return new Promise((res, rej) => {
        const tryOnce = () => {
            const s = connect(port, "127.0.0.1");
            s.on("connect", () => {
                s.end();
                res();
            });
            s.on("error", () => {
                s.destroy();
                if (Date.now() - start > ms) rej(new Error("proxy not ready"));
                else setTimeout(tryOnce, 200);
            });
        };
        tryOnce();
    });
}

function cmdSetup(args) {
    const port = Number(opt(args, "--port", DEFAULT_PORT));
    const mitm = spawnSync("which", ["mitmdump"], {encoding: "utf8"});
    if (mitm.status !== 0) fail("mitmdump not found; install with: brew install mitmproxy");
    if (!existsSync(CA)) fail(`CA missing at ${CA}; start mitmdump once to generate it`);
    mkdirSync(OUT_DIR, {recursive: true, mode: 0o700});
    writeState({port, ca: CA, setupAt: new Date().toISOString(), trusted: false});
    info(`output dir ${OUT_DIR}`);
    info(`CA ${CA}`);
    const trustCmd = `sudo security add-trusted-cert -d -p ssl -p basic -k /Library/Keychains/System.keychain ${CA}`;
    if (flag(args, "--trust")) {
        const r = spawnSync("sudo", ["security", "add-trusted-cert", "-d", "-p", "ssl", "-p", "basic",
            "-k", "/Library/Keychains/System.keychain", CA], {stdio: "inherit"});
        if (r.status !== 0) fail(`trust failed; run manually: ${trustCmd}`);
        writeState({trusted: true});
        info("CA trusted in System keychain");
    } else {
        info("to intercept TLS, trust the CA once:");
        console.log(`  ${trustCmd}`);
        info("or re-run with --trust");
        info("Node-based clients also work without system trust via NODE_EXTRA_CA_CERTS (set by `run`)");
    }
}

function cmdTeardown(args) {
    try {
        const pid = Number(readFileSync(PID_FILE, "utf8").trim());
        if (pid) {
            try {
                process.kill(pid);
                info(`stopped proxy pid ${pid}`);
            } catch {
            }
        }
    } catch {
    }
    try {
        rmSync(PID_FILE, {force: true});
    } catch {
    }
    let cert = CA, wasTrusted = false;
    try {
        const st = JSON.parse(readFileSync(STATE_FILE, "utf8"));
        cert = st.ca || CA;
        wasTrusted = st.trusted === true;
    } catch {
    }
    if (wasTrusted) {
        const r = spawnSync("sudo", ["security", "remove-trusted-cert", "-d", cert], {stdio: "inherit"});
        if (r.status !== 0) info("trust removal failed; remove manually in Keychain Access");
        else info("removed CA trust");
    } else {
        info("CA trust was not installed by setup; nothing to remove");
    }
    try {
        rmSync(STATE_FILE, {force: true});
    } catch {
    }
    if (flag(args, "--purge")) {
        rmSync(OUT_DIR, {recursive: true, force: true});
        info("purged captures");
    }
    info("teardown done (CA files kept; captures kept unless --purge)");
}

async function cmdRun(args) {
    const sep = args.indexOf("--");
    const child = sep >= 0 ? args.slice(sep + 1) : [];
    if (child.length === 0) fail("usage: node scripts/capture.mjs run -- <client> [args]");
    const runArgs = sep >= 0 ? args.slice(0, sep) : args;
    const port = Number(opt(runArgs, "--port", DEFAULT_PORT));
    const local = opt(runArgs, "--local", null);
    const showSecrets = flag(runArgs, "--show-secrets");
    if (!existsSync(CA)) fail(`CA missing at ${CA}; run setup first`);
    mkdirSync(OUT_DIR, {recursive: true, mode: 0o700});
    const runId = `${Date.now().toString(36)}-${process.pid}`;

    const mitmArgs = ["--listen-port", String(port), "-s", ADDON, "--set", "confdir=" + join(homedir(), ".mitmproxy")];
    if (local) mitmArgs.push("--mode", `local:${local}`);
    const mitm = spawn("mitmdump", mitmArgs, {
        cwd: ROOT,
        env: {
            ...process.env,
            CAPTURE_DIR: OUT_DIR,
            CAPTURE_RUN_ID: runId,
            CAPTURE_SHOW_SECRETS: showSecrets ? "1" : ""
        },
        stdio: ["ignore", "pipe", "pipe"],
    });
    writeFileSync(PID_FILE, String(mitm.pid));
    mitm.stdout.on("data", (d) => process.stdout.write(`[mitm] ${d}`));
    mitm.stderr.on("data", (d) => process.stderr.write(`[mitm] ${d}`));
    mitm.on("exit", (c) => {
        if (c !== 0 && c !== null) console.error(`capture: mitmdump exited ${c}`);
    });

    const stop = () => {
        try {
            mitm.kill();
        } catch {
        }
        try {
            rmSync(PID_FILE, {force: true});
        } catch {
        }
    };
    process.on("SIGINT", () => {
        stop();
        process.exit(130);
    });
    process.on("SIGTERM", () => {
        stop();
        process.exit(143);
    });

    try {
        await waitPort(port);
    } catch {
        stop();
        fail("mitmdump did not listen; is the port busy?");
    }
    info(`proxy on 127.0.0.1:${port}${local ? ` (local:${local})` : ""}${showSecrets ? " (secrets RAW)" : " (secrets masked)"}`);

    const proxy = `http://127.0.0.1:${port}`;
    const kid = spawn(child[0], child.slice(1), {
        stdio: "inherit",
        env: {
            ...process.env,
            HTTP_PROXY: proxy, HTTPS_PROXY: proxy, http_proxy: proxy, https_proxy: proxy,
            NO_PROXY: "", no_proxy: "",
            NODE_EXTRA_CA_CERTS: CA, SSL_CERT_FILE: CA, REQUESTS_CA_BUNDLE: CA, CURL_CA_BUNDLE: CA,
        },
    });
    const code = await new Promise((res) => kid.on("close", res));
    stop();
    info(`client exited ${code}; captures in ${OUT_DIR}`);
    process.exit(code ?? 0);
}

const [cmd = "--help", ...rest] = process.argv.slice(2);
if (cmd === "setup") cmdSetup(rest);
else if (cmd === "teardown") cmdTeardown(rest);
else if (cmd === "run") await cmdRun(rest);
else {
    console.log("usage:\n  node scripts/capture.mjs setup [--port N] [--trust]\n  node scripts/capture.mjs run [--port N] [--show-secrets] [--local name] -- <client> [args]\n  node scripts/capture.mjs teardown [--purge]");
}
