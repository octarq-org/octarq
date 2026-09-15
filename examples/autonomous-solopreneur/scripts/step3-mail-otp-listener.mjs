#!/usr/bin/env node
// Step 3: Monitor Dedicated Business Mailbox & Extract SaaS Verification OTP over Octarq MCP

import { OctarqMCPClient } from '../src/mcp-client.mjs';
import { listenForOnboardingOTP } from '../src/mail-otp.mjs';
import { banner, section, info, success, warn } from '../src/utils.mjs';

async function main() {
  banner('STEP 3: BUSINESS MAILBOX & OTP LISTENER', 'Listen to incoming emails and extract OTP verification codes over MCP');

  const client = new OctarqMCPClient();
  await client.init();

  section(3, 'Inbox Polling & Automated OTP Interception', 'Inspecting incoming emails for SaaS confirmation codes...');

  // If running in simulation, ensure there is an incoming OTP email to intercept
  if (!client.isLive) {
    info('Simulating incoming registration email from Cloudflare Zero Trust...');
    client.injectSimulatedEmail({
      from: 'no-reply@notify.cloudflare.com',
      subject: 'Your Cloudflare verification token is 529184',
      snippet: 'Please verify your email address to activate your Autonomous Solopreneur tunnel. Your code is 529184. It expires in 15 minutes.',
    });
  }

  const result = await listenForOnboardingOTP(client);

  if (result.found) {
    success(`Successfully extracted verification payload!`);
    console.log(`\nStructured Agent Output:`);
    console.log(JSON.stringify({
      status: 'VERIFICATION_RECEIVED',
      sender: result.email.from,
      otpCode: result.otp,
      magicLink: result.magicLink,
      action: 'Ready to auto-fill into external SaaS registration form',
    }, null, 2));
  } else {
    warn('No pending verification email found in mailbox.');
  }
}

main().catch(console.error);
