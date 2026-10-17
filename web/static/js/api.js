// api.js: the only place that talks to the JSON API. Every mutation carries
// the CSRF header the server requires; fetch adds the Origin header itself.

/** An error answer of the API: status, machine code and a message for people. */
export class ApiError extends Error {
  constructor(status, code, message, body) {
    super(message || `Request failed with status ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.code = code || "error";
    this.body = body || {};
  }
}

function headers(method, hasBody) {
  const h = { Accept: "application/json" };
  if (hasBody) h["Content-Type"] = "application/json";
  if (method !== "GET" && method !== "HEAD") h["X-GPW-CSRF"] = "1";
  return h;
}

/** Throws an ApiError for a response that is not 2xx. */
async function check(res) {
  if (res.ok) return res;
  let body = {};
  try {
    body = await res.json();
  } catch {
    // Not JSON: a proxy error page. The status says enough.
  }
  const retry = res.headers.get("Retry-After");
  if (retry) body.retryAfter = Number(retry);
  throw new ApiError(res.status, body.code, body.error, body);
}

/**
 * Calls the API and returns the parsed JSON.
 * @param {string} method
 * @param {string} path
 * @param {unknown} [body]
 * @param {{signal?: AbortSignal, keepalive?: boolean}} [opts] keepalive lets a save finish while the page closes
 */
export async function api(method, path, body, opts = {}) {
  const hasBody = body !== undefined;
  let res;
  try {
    res = await fetch(path, {
      method,
      headers: headers(method, hasBody),
      body: hasBody ? JSON.stringify(body) : undefined,
      credentials: "same-origin",
      signal: opts.signal,
      keepalive: opts.keepalive || false,
    });
  } catch (err) {
    if (err.name === "AbortError") throw err;
    throw new ApiError(0, "network", "The site cannot be reached. Check your connection.");
  }
  await check(res);
  if (res.status === 204) return {};
  return res.json();
}

/**
 * Posts a request and calls onEvent for every line of the NDJSON answer.
 * Resolves when the stream ends.
 */
export async function apiStream(path, body, onEvent, opts = {}) {
  let res;
  try {
    res = await fetch(path, {
      method: "POST",
      headers: { ...headers("POST", true), Accept: "application/x-ndjson" },
      body: JSON.stringify(body),
      credentials: "same-origin",
      signal: opts.signal,
    });
  } catch (err) {
    if (err.name === "AbortError") throw err;
    throw new ApiError(0, "network", "The site cannot be reached. Check your connection.");
  }
  await check(res);
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let nl;
    while ((nl = buf.indexOf("\n")) >= 0) {
      const line = buf.slice(0, nl).trim();
      buf = buf.slice(nl + 1);
      if (line) onEvent(JSON.parse(line));
    }
  }
  const rest = (buf + decoder.decode()).trim();
  if (rest) onEvent(JSON.parse(rest));
}

/** Shows a message in a status element; kind is "error", "success" or "". */
export function setStatus(el, message, kind = "") {
  if (!el) return;
  el.textContent = message;
  if (kind) el.dataset.kind = kind;
  else delete el.dataset.kind;
}

/**
 * The message to show for any error. context names the passkey task, so
 * the browser's few error names become advice that fits: "login",
 * "register" (a new account) or "add" (a passkey for a signed-in account).
 */
export function errorMessage(err, context = "") {
  if (err instanceof ApiError) return err.message;
  if (err && err.name === "NotAllowedError") {
    if (context === "login") return "You closed the passkey prompt, it timed out, or this device has no passkey for Go Practice. Try again, use your phone, or recover your account.";
    return "You closed the passkey prompt, or it timed out. Try again when you are ready.";
  }
  if (err && err.name === "InvalidStateError") {
    if (context === "add") return "This device already has a passkey for your account. Add the new one on another device or a security key.";
    return "This device already holds a passkey for this account. Sign in with it.";
  }
  if (err && err.name === "SecurityError") return "The browser refused the passkey for this site address. Open the site at its own address and try again.";
  return (err && err.message) || "The request failed. Try again.";
}
