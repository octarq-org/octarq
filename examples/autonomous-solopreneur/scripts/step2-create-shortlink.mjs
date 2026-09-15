#!/usr/bin/env node
// Step 2: Auto-configure Subdomain & Marketing Shortlink with UTM Tags over Octarq MCP

import { OctarqMCPClient } from '../src/mcp-client.mjs';
import { createMarketingShortlink } from '../src/shortlink.mjs';
import { banner, section, success } from '../src/utils.mjs';

async function main() {
  banner('STEP 2: MARKETING SHORTLINK CREATION', 'Provision branded subdomain links with UTM tracking over MCP');

  const client = new OctarqMCPClient();
  await client.init();

  section(2, 'Subdomain & UTM Marketing Link Automation', 'Invoking create_shortlink declarative endpoint...');

  const destination = process.argv[2] || 'https://solopreneur.dev/launch';
  const slug = process.argv[3] || 'launch';
  const host = process.argv[4] || process.env.SOLOPRENEUR_SHORTLINK_HOST || 'go.solopreneur.dev';

  const result = await createMarketingShortlink(client, {
    destination,
    slug,
    host,
    title: 'Autonomous Launch Campaign',
    utm: {
      utm_source: 'twitter',
      utm_medium: 'agent_broadcast',
      utm_campaign: 'solopreneur_beta',
      utm_content: 'hero_cta',
    },
  });

  console.log('\nResult Summary:');
  console.log(`- Short URL:   ${result.shortUrl}`);
  console.log(`- Target URL:  ${result.target}`);
  console.log(`- Verified:    ${result.verified ? 'YES (Listed in workspace links)' : 'PENDING'}`);

  success('Step 2 completed: Marketing shortlink ready for autonomous distribution.');
}

main().catch(console.error);
