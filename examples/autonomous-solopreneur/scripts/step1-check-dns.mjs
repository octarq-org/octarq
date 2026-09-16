#!/usr/bin/env node
// Step 1: Detect Domain Sending Reputation & DNS Configuration over Octarq Remote MCP

import { OctarqMCPClient } from '../src/mcp-client.mjs';
import { auditDomainDNS, printDnsAuditReport } from '../src/dns-checker.mjs';
import { banner, section, info, warn } from '../src/utils.mjs';

async function main() {
  banner('STEP 1: DNS & REPUTATION AUDIT', 'Inspect domain deliverability, SPF, DKIM, DMARC over Remote MCP');

  const client = new OctarqMCPClient();
  await client.init();

  section(1, 'Domain & DNS Deliverability Inspection', 'Querying list_domains via Octarq MCP endpoint...');

  const domains = await client.callTool('list_domains');

  if (!domains || domains.length === 0) {
    warn('No domains registered in this Octarq workspace.');
    return;
  }

  const targetDomainName = process.argv[2] || process.env.SOLOPRENEUR_DOMAIN || domains[0].name;
  const targetDomain = domains.find((d) => d.name === targetDomainName) || domains[0];

  info(`Analyzing domain configuration for: ${targetDomain.name}`);
  const audit = auditDomainDNS(targetDomain);
  printDnsAuditReport(audit);
}

main().catch(console.error);
