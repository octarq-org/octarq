// Automated test suite for Autonomous Solopreneur Blueprint modules

import { test, describe } from 'node:test';
import assert from 'node:assert';
import {
  generateEd25519Keypair,
  issueLicense,
  verifyLicense,
  canonicalJson,
  composeLicenseNotificationEmail,
} from '../src/license.mjs';
import { extractOTPFromText } from '../src/mail-otp.mjs';
import { buildUtmUrl } from '../src/shortlink.mjs';
import { auditDomainDNS } from '../src/dns-checker.mjs';
import { OctarqMCPClient } from '../src/mcp-client.mjs';

describe('Ed25519 Licensing Cryptography', () => {
  test('generates valid Ed25519 keypair in PEM and raw base64', () => {
    const keypair = generateEd25519Keypair();
    assert.ok(keypair.privateKeyPem.includes('BEGIN PRIVATE KEY'));
    assert.ok(keypair.publicKeyPem.includes('BEGIN PUBLIC KEY'));
    assert.strictEqual(typeof keypair.publicKeyRawBase64, 'string');
    assert.ok(keypair.publicKeyRawBase64.length > 0);
  });

  test('canonicalizes JSON deterministically regardless of key insertion order', () => {
    const a = { z: 1, a: 2, m: { b: 3, a: 4 } };
    const b = { a: 2, m: { a: 4, b: 3 }, z: 1 };
    assert.strictEqual(canonicalJson(a), canonicalJson(b));
    assert.strictEqual(canonicalJson(a), '{"a":2,"m":{"a":4,"b":3},"z":1}');
  });

  test('issues and verifies valid offline Ed25519 license', () => {
    const keypair = generateEd25519Keypair();
    const payload = {
      customer: { email: 'alice@example.com', name: 'Alice' },
      plan: 'pro_monthly',
      seats: 5,
    };

    const issued = issueLicense(payload, keypair.privateKeyPem);
    assert.ok(issued.token.startsWith('OCTARQ-LIC-v1.'));

    const verification = verifyLicense(issued.token, keypair.publicKeyPem);
    assert.strictEqual(verification.valid, true);
    assert.strictEqual(verification.payload.customer.email, 'alice@example.com');
    assert.strictEqual(verification.payload.plan, 'pro_monthly');
    assert.strictEqual(verification.payload.seats, 5);
  });

  test('rejects tampered license token', () => {
    const keypair = generateEd25519Keypair();
    const issued = issueLicense({ customer: { email: 'bob@example.com' } }, keypair.privateKeyPem);

    const parts = issued.token.split('.');
    // Tamper with payload (change base64 content)
    const tamperedPayload = Buffer.from(JSON.stringify({ hacked: true })).toString('base64url');
    const tamperedToken = `${parts[0]}.${tamperedPayload}.${parts[2]}`;

    const verification = verifyLicense(tamperedToken, keypair.publicKeyPem);
    assert.strictEqual(verification.valid, false);
    assert.ok(verification.reason.includes('Cryptographic signature verification failed'));
  });

  test('rejects expired license token', () => {
    const keypair = generateEd25519Keypair();
    const expiredDate = new Date(Date.now() - 3600000).toISOString();
    const issued = issueLicense({ expiresAt: expiredDate }, keypair.privateKeyPem);

    const verification = verifyLicense(issued.token, keypair.publicKeyPem);
    assert.strictEqual(verification.valid, false);
    assert.ok(verification.reason.includes('expired'));
  });

  test('composes customer notification email containing token and details', () => {
    const keypair = generateEd25519Keypair();
    const issued = issueLicense({ customer: { email: 'carol@test.io', name: 'Carol' } }, keypair.privateKeyPem);
    const email = composeLicenseNotificationEmail(issued);

    assert.ok(email.subject.includes(issued.payload.licenseId));
    assert.ok(email.bodyText.includes('Carol'));
    assert.ok(email.bodyText.includes(issued.token));
  });
});

describe('Mailbox OTP Extractor', () => {
  test('extracts 6-digit OTP code with keyword', () => {
    const text1 = 'Your Cloudflare verification code is 849201. Do not share this with anyone.';
    const res1 = extractOTPFromText(text1);
    assert.strictEqual(res1.code, '849201');
    assert.strictEqual(res1.confidence, 'high');

    const text2 = '【GitHub】您的验证码为 492018，有效期 10 分钟。';
    const res2 = extractOTPFromText(text2);
    assert.strictEqual(res2.code, '492018');
    assert.strictEqual(res2.confidence, 'high');
  });

  test('extracts standalone 6-digit numeric OTP code', () => {
    const text = 'Login confirmation for your workspace: 938201';
    const res = extractOTPFromText(text);
    assert.strictEqual(res.code, '938201');
  });

  test('extracts magic link when numeric code is absent', () => {
    const text = 'Click here to activate your account: https://auth.provider.com/verify?token=xyz890123';
    const res = extractOTPFromText(text);
    assert.strictEqual(res.code, null);
    assert.strictEqual(res.magicLink, 'https://auth.provider.com/verify?token=xyz890123');
  });

  test('handles empty or irrelevant text safely', () => {
    const res = extractOTPFromText('Hello, this is a general newsletter about AI agents.');
    assert.strictEqual(res.code, null);
    assert.strictEqual(res.confidence, 'none');
  });
});

describe('Marketing Shortlink UTM Builder', () => {
  test('injects default and custom UTM parameters into URL', () => {
    const url = buildUtmUrl('https://solopreneur.dev/launch', {
      utm_source: 'linkedin',
      utm_campaign: 'ai_launch_2026',
    });

    const parsed = new URL(url);
    assert.strictEqual(parsed.origin, 'https://solopreneur.dev');
    assert.strictEqual(parsed.pathname, '/launch');
    assert.strictEqual(parsed.searchParams.get('utm_source'), 'linkedin');
    assert.strictEqual(parsed.searchParams.get('utm_campaign'), 'ai_launch_2026');
    assert.strictEqual(parsed.searchParams.get('utm_medium'), 'agent_broadcast');
  });
});

describe('DNS Deliverability Auditor', () => {
  test('evaluates healthy domain with 100/100 reputation score', () => {
    const domain = {
      name: 'solopreneur.dev',
      forMail: true,
      forLink: true,
    };
    const audit = auditDomainDNS(domain);
    assert.strictEqual(audit.reputationScore, 100);
    assert.strictEqual(audit.readyForMail, true);
    assert.strictEqual(audit.readyForLinks, true);
    assert.strictEqual(audit.recommendations.length, 0);
  });

  test('penalizes missing email or link flags', () => {
    const domain = {
      name: 'broken.dev',
      forMail: false,
      forLink: true,
    };
    const audit = auditDomainDNS(domain);
    assert.strictEqual(audit.reputationScore, 70);
    assert.strictEqual(audit.readyForMail, false);
    assert.ok(audit.recommendations.length > 0);
  });
});

describe('Octarq MCP Client Simulation', () => {
  test('executes MCP tool suite in simulated environment', async () => {
    const client = new OctarqMCPClient({ mode: 'simulate' });
    await client.init();

    const domains = await client.callTool('list_domains');
    assert.ok(Array.isArray(domains));
    assert.ok(domains.length > 0);

    const created = await client.callTool('create_shortlink', {
      destination: 'https://example.com/test',
      slug: 'test-slug',
      host: 'go.example.com',
    });
    assert.strictEqual(created.success, true);
    assert.strictEqual(created.link.slug, 'test-slug');

    const links = await client.callTool('list_links');
    assert.ok(links.some((l) => l.slug === 'test-slug'));

    const mailboxes = await client.callTool('list_mailboxes');
    assert.ok(Array.isArray(mailboxes));

    const emails = await client.callTool('list_emails');
    assert.ok(Array.isArray(emails));
  });
});
