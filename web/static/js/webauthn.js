// webauthn.js: passkey ceremonies in the browser. The server sends the
// options as JSON with base64url byte fields; navigator.credentials wants
// ArrayBuffers and returns them. These helpers convert both ways, so
// every page only calls createPasskey or getPasskey.

/** Encodes bytes as unpadded base64url. */
export function toBase64url(buf) {
  const bytes = new Uint8Array(buf);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** Decodes base64url (padded or not) to an ArrayBuffer. */
export function fromBase64url(s) {
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/");
  const bin = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

/** Reports whether the browser has WebAuthn at all. */
export function supported() {
  return typeof window.PublicKeyCredential === "function" && !!navigator.credentials;
}

/** Reports whether passkey autofill (conditional mediation) works here. */
export async function autofillSupported() {
  if (!supported() || typeof PublicKeyCredential.isConditionalMediationAvailable !== "function") return false;
  try {
    return await PublicKeyCredential.isConditionalMediationAvailable();
  } catch {
    return false;
  }
}

function descriptors(list) {
  return (list || []).map((d) => ({ ...d, id: fromBase64url(d.id) }));
}

function creationOptions(pk) {
  return {
    ...pk,
    challenge: fromBase64url(pk.challenge),
    user: { ...pk.user, id: fromBase64url(pk.user.id) },
    excludeCredentials: descriptors(pk.excludeCredentials),
  };
}

function requestOptions(pk) {
  return {
    ...pk,
    challenge: fromBase64url(pk.challenge),
    allowCredentials: descriptors(pk.allowCredentials),
  };
}

/** The JSON form of a PublicKeyCredential that the server parses. */
function credentialJSON(cred) {
  const r = cred.response;
  const response = { clientDataJSON: toBase64url(r.clientDataJSON) };
  if (r.attestationObject) {
    response.attestationObject = toBase64url(r.attestationObject);
    if (typeof r.getTransports === "function") response.transports = r.getTransports();
  } else {
    response.authenticatorData = toBase64url(r.authenticatorData);
    response.signature = toBase64url(r.signature);
    if (r.userHandle) response.userHandle = toBase64url(r.userHandle);
  }
  return {
    id: cred.id,
    rawId: toBase64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment || undefined,
    clientExtensionResults: cred.getClientExtensionResults ? cred.getClientExtensionResults() : {},
    response,
  };
}

/**
 * Creates a passkey from the options of a begin endpoint
 * ({publicKey: {...}}) and returns the credential JSON for finish.
 */
export async function createPasskey(options) {
  if (!supported()) throw new Error("This browser does not support passkeys.");
  const cred = await navigator.credentials.create({ publicKey: creationOptions(options.publicKey) });
  return credentialJSON(cred);
}

/**
 * Asks for a passkey. With conditional, the request waits in the autofill
 * list of an input with autocomplete="webauthn" until the user picks one;
 * abort it with signal before starting another request.
 */
export async function getPasskey(options, { signal, conditional = false } = {}) {
  if (!supported()) throw new Error("This browser does not support passkeys.");
  const req = { publicKey: requestOptions(options.publicKey), signal };
  if (conditional) req.mediation = "conditional";
  const cred = await navigator.credentials.get(req);
  return credentialJSON(cred);
}
