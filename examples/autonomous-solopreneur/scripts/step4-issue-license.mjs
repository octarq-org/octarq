#!/usr/bin/env node
// Step 4: Simulate Transaction, Sign Offline Ed25519 License & Draft Customer Email

import {
  generateEd25519Keypair,
  issueLicense,
  verifyLicense,
  composeLicenseNotificationEmail,
} from '../src/license.mjs';
import { banner, section, success, info, colors } from '../src/utils.mjs';

async function main() {
  banner('STEP 4: ED25519 OFFLINE LICENSING & DISPATCH', 'Mint cryptographically signed offline licenses and generate delivery emails');

  section(4, 'Ed25519 Keypair & License Generation', 'Generating keypair and signing offline license payload...');

  // 1. Keypair generation (or load from environment)
  const keypair = generateEd25519Keypair();
  info(`Public Key (SPKI Raw Base64): ${colors.brightCyan}${keypair.publicKeyRawBase64}${colors.reset}`);

  // 2. Mock incoming transaction event
  const buyerEmail = process.argv[2] || 'sarah.connor@cyberdyne.ai';
  const plan = process.argv[3] || 'lifetime_pro';

  info(`Processing checkout completion for customer: ${buyerEmail} (Plan: ${plan})`);

  const licensePayload = {
    customer: {
      email: buyerEmail,
      name: 'Sarah Connor',
      org: 'Cyberdyne Systems AI Lab',
    },
    plan,
    seats: 3,
    features: ['mcp_unlimited', 'custom_subdomains', 'unattended_ops', 'priority_support'],
    expiresAt: null, // Lifetime perpetual
  };

  // 3. Issue and sign license
  const issued = issueLicense(licensePayload, keypair.privateKeyPem);
  success(`Minted License ID: ${colors.bold}${issued.payload.licenseId}${colors.reset}`);
  console.log(`\n${colors.dim}Armored License Token:${colors.reset}\n${colors.brightYellow}${issued.token}${colors.reset}\n`);

  // 4. Offline Verification Test (Using only the Public Key)
  info('Testing 100% offline verification using Vendor Public Key (zero network calls)...');
  const verification = verifyLicense(issued.token, keypair.publicKeyPem);

  if (verification.valid) {
    success(`Offline Cryptographic Signature VERIFIED!`);
    console.log(`- License ID:  ${verification.payload.licenseId}`);
    console.log(`- Customer:    ${verification.payload.customer.email}`);
    console.log(`- Plan:        ${verification.payload.plan}`);
    console.log(`- Status:      Valid & Authenticated`);
  } else {
    throw new Error(`Verification failed: ${verification.reason}`);
  }

  // 5. Compose customer delivery notification
  section('4b', 'Customer Notification & Dispatch', 'Generating fulfillment notification email...');
  const email = composeLicenseNotificationEmail(issued);

  console.log(`${colors.dim}Subject: ${email.subject}${colors.reset}`);
  console.log(`${colors.dim}──────────────────────────────────────────────────────────────────${colors.reset}`);
  console.log(email.bodyText);
  console.log(`${colors.dim}──────────────────────────────────────────────────────────────────${colors.reset}\n`);

  success('Customer fulfillment package prepared for automated SMTP dispatch.');
}

main().catch(console.error);
