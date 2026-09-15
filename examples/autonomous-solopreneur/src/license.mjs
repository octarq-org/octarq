// Ed25519 Cryptographic Licensing Subsystem
// Enables autonomous solopreneurs to issue, sign, and verify tamper-proof software licenses
// completely offline without relying on phone-home servers or third-party DRM SaaS.

import crypto from 'node:crypto';
import { colors, success, info, warn, error } from './utils.mjs';

/**
 * Generates an Ed25519 asymmetric keypair in PKCS#8 / SPKI PEM format.
 * @returns {{ privateKeyPem: string, publicKeyPem: string, publicKeyRawBase64: string }}
 */
export function generateEd25519Keypair() {
  const { privateKey, publicKey } = crypto.generateKeyPairSync('ed25519');

  const privateKeyPem = privateKey.export({ type: 'pkcs8', format: 'pem' });
  const publicKeyPem = publicKey.export({ type: 'spki', format: 'pem' });

  // Export raw 32-byte public key in base64url for compact embedding
  const rawSpki = publicKey.export({ type: 'spki', format: 'der' });
  // Ed25519 SPKI has a 12-byte header (302a300506032b6570032100), followed by 32 bytes raw key
  const rawKey = rawSpki.subarray(rawSpki.length - 32);
  const publicKeyRawBase64 = rawKey.toString('base64url');

  return {
    privateKeyPem,
    publicKeyPem,
    publicKeyRawBase64,
  };
}

/**
 * Serializes an object to deterministic canonical JSON (sorted keys, no extra spaces)
 * @param {object} obj
 * @returns {string}
 */
export function canonicalJson(obj) {
  if (obj === null || typeof obj !== 'object') {
    return JSON.stringify(obj);
  }
  if (Array.isArray(obj)) {
    return '[' + obj.map(canonicalJson).join(',') + ']';
  }
  const keys = Object.keys(obj).sort();
  return '{' + keys.map((k) => JSON.stringify(k) + ':' + canonicalJson(obj[k])).join(',') + '}';
}

/**
 * Issues and signs an offline license token with an Ed25519 private key.
 * @param {object} payload
 * @param {string | crypto.KeyObject} privateKey
 * @returns {{ token: string, payload: object, signatureHex: string }}
 */
export function issueLicense(payload, privateKey) {
  const defaults = {
    licenseId: `lic_${crypto.randomBytes(6).toString('hex')}`,
    product: 'Octarq Solopreneur Suite',
    plan: 'commercial_unlimited',
    seats: 1,
    features: ['mcp_access', 'custom_subdomains', 'unattended_ops'],
    issuedAt: new Date().toISOString(),
    expiresAt: null, // Perpetual license
  };

  const fullPayload = { ...defaults, ...payload };
  const canonicalData = Buffer.from(canonicalJson(fullPayload), 'utf8');

  // Sign canonical payload using Ed25519 (null algorithm specifies raw Ed25519 RFC 8032)
  const signature = crypto.sign(null, canonicalData, privateKey);

  const payloadB64 = Buffer.from(JSON.stringify(fullPayload), 'utf8').toString('base64url');
  const signatureB64 = signature.toString('base64url');

  const token = `OCTARQ-LIC-v1.${payloadB64}.${signatureB64}`;

  return {
    token,
    payload: fullPayload,
    signatureHex: signature.toString('hex'),
  };
}

/**
 * Completely offline verification of an Ed25519 license token against the vendor's public key.
 * @param {string} token
 * @param {string | crypto.KeyObject} publicKey
 * @returns {{ valid: boolean, payload?: object, reason?: string }}
 */
export function verifyLicense(token, publicKey) {
  if (!token || typeof token !== 'string') {
    return { valid: false, reason: 'Empty or invalid token format' };
  }

  const parts = token.trim().split('.');
  if (parts.length !== 3 || parts[0] !== 'OCTARQ-LIC-v1') {
    return { valid: false, reason: 'Invalid license envelope prefix (expected OCTARQ-LIC-v1.<payload>.<sig>)' };
  }

  const [_, payloadB64, signatureB64] = parts;

  let payload;
  try {
    const rawPayload = Buffer.from(payloadB64, 'base64url').toString('utf8');
    payload = JSON.parse(rawPayload);
  } catch (err) {
    return { valid: false, reason: 'Malformed payload JSON in token' };
  }

  let signature;
  try {
    signature = Buffer.from(signatureB64, 'base64url');
  } catch (err) {
    return { valid: false, reason: 'Malformed signature encoding' };
  }

  // Canonicalize parsed payload and verify
  const canonicalData = Buffer.from(canonicalJson(payload), 'utf8');

  try {
    const verified = crypto.verify(null, canonicalData, publicKey, signature);
    if (!verified) {
      return { valid: false, reason: 'Cryptographic signature verification failed (tampered payload or key mismatch)' };
    }
  } catch (err) {
    return { valid: false, reason: `Verification error: ${err.message}` };
  }

  // Check expiration date if present
  if (payload.expiresAt) {
    const expiry = new Date(payload.expiresAt);
    if (Date.now() > expiry.getTime()) {
      return { valid: false, payload, reason: `License expired on ${payload.expiresAt}` };
    }
  }

  return { valid: true, payload };
}

/**
 * Generates formatted customer notification message for email delivery.
 * @param {object} licenseInfo
 * @returns {{ subject: string, bodyText: string }}
 */
export function composeLicenseNotificationEmail(licenseInfo) {
  const { payload, token } = licenseInfo;
  const customerName = payload.customer?.name || payload.customer?.email || 'Valued Customer';

  const subject = `Your ${payload.product} License Key is Ready (${payload.licenseId})`;

  const bodyText = `
Hello ${customerName},

Thank you for your purchase! Your offline-activated license for ${payload.product} has been successfully minted.

══════════════════════════════════════════════════════════════════
LICENSED DETAILS
══════════════════════════════════════════════════════════════════
License ID:    ${payload.licenseId}
Plan Tier:     ${payload.plan}
Seats:         ${payload.seats}
Customer:      ${payload.customer?.email} (${payload.customer?.org || 'Individual'})
Issued Date:   ${payload.issuedAt}
Expiry:        ${payload.expiresAt || 'Perpetual (No Expiration)'}
Features:      ${(payload.features || []).join(', ')}

══════════════════════════════════════════════════════════════════
OFFLINE ACTIVATION TOKEN (Ed25519 Cryptographically Signed)
══════════════════════════════════════════════════════════════════
${token}

══════════════════════════════════════════════════════════════════
ACTIVATION INSTRUCTIONS
══════════════════════════════════════════════════════════════════
1. Copy the token above.
2. In your Octarq dashboard or CLI, navigate to:
   Settings -> License -> Enter License Key
3. Paste the token. The binary validates the cryptographic signature
   locally with zero network calls and instantly unlocks your tier.

Thank you for choosing Octarq!
--
Octarq Autonomous Ops Daemon
  `.trim();

  return { subject, bodyText };
}
