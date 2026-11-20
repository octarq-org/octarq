
import { colors, success, info, warn } from './utils.mjs';

export function extractOTPFromText(text) {
  if (!text) return { code: null, magicLink: null, confidence: 'none' };

  const keywordRegex = /(?:code|otp|token|pin|passcode|verification\s*code|验证码)[^\d]{0,15}(\b\d{4,8}\b)/i;
  const keywordMatch = text.match(keywordRegex);
  if (keywordMatch && keywordMatch[1]) {
    return {
      code: keywordMatch[1],
      magicLink: extractMagicLink(text),
      confidence: 'high',
    };
  }

  const sixDigitRegex = /\b(\d{6})\b/;
  const sixDigitMatch = text.match(sixDigitRegex);
  if (sixDigitMatch && sixDigitMatch[1]) {
    return {
      code: sixDigitMatch[1],
      magicLink: extractMagicLink(text),
      confidence: 'medium',
    };
  }

  const magicLink = extractMagicLink(text);
  if (magicLink) {
    return {
      code: null,
      magicLink,
      confidence: 'high',
    };
  }

  return { code: null, magicLink: null, confidence: 'none' };
}

function extractMagicLink(text) {
  const urlRegex = /(https?:\/\/[^\s"'<>]*(?:verify|confirm|activate|auth|signup)[^\s"'<>]*)/i;
  const match = text.match(urlRegex);
  return match ? match[1] : null;
}

export async function listenForOnboardingOTP(mcpClient, options = {}) {
  info('Querying active mailboxes via MCP (list_mailboxes)...');
  const mailboxes = await mcpClient.callTool('list_mailboxes');
  const targetMailbox = (mailboxes && mailboxes[0]) ? mailboxes[0].address : 'ops@solopreneur.dev';

  info(`Listening on business mailbox: ${colors.bold}${targetMailbox}${colors.reset}`);

  const emails = await mcpClient.callTool('list_emails', {
    mailboxId: options.mailboxId || (mailboxes && mailboxes[0] ? mailboxes[0].id : 1),
    limit: 10,
    unreadOnly: false,
  });

  if (!emails || emails.length === 0) {
    warn('No emails in inbox.');
    return { found: false };
  }

  for (const email of emails) {
    const combinedContent = `${email.subject || ''} ${email.snippet || ''}`;
    const extraction = extractOTPFromText(combinedContent);

    if (extraction.code || extraction.magicLink) {
      success(`Intercepted verification message from ${colors.brightCyan}${email.from}${colors.reset}`);
      console.log(`   Subject:   ${email.subject}`);
      if (extraction.code) {
        console.log(`   OTP Code:  ${colors.bold}${colors.brightGreen}${extraction.code}${colors.reset} (Confidence: ${extraction.confidence})`);
      }
      if (extraction.magicLink) {
        console.log(`   Magic Link: ${colors.brightBlue}${extraction.magicLink}${colors.reset}`);
      }

      return {
        found: true,
        email,
        otp: extraction.code,
        magicLink: extraction.magicLink,
      };
    }
  }

  warn('No OTP code detected in recent incoming messages.');
  return { found: false };
}
