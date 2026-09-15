// Marketing shortlink configurator with UTM parameter injection
// Exposes automated campaign link creation over Octarq's declarative shortlink MCP endpoint.

import { success, info } from './utils.mjs';

/**
 * Attaches standard UTM marketing query parameters to any destination URL.
 * @param {string} destination
 * @param {object} utmParams
 * @returns {string}
 */
export function buildUtmUrl(destination, utmParams = {}) {
  const url = new URL(destination);
  const defaults = {
    utm_source: 'twitter',
    utm_medium: 'agent_broadcast',
    utm_campaign: 'autonomous_solopreneur_v1',
    utm_content: 'ai_ops_hero',
  };

  const finalParams = { ...defaults, ...utmParams };
  for (const [k, v] of Object.entries(finalParams)) {
    if (v) url.searchParams.set(k, v);
  }

  return url.toString();
}

/**
 * Automates creating a trackable branded shortlink via MCP.
 * @param {import('./mcp-client.mjs').OctarqMCPClient} mcpClient
 * @param {object} options
 */
export async function createMarketingShortlink(mcpClient, options = {}) {
  const destination = options.destination || 'https://solopreneur.dev/launch';
  const slug = options.slug || 'launch';
  const host = options.host || process.env.SOLOPRENEUR_SHORTLINK_HOST || 'go.solopreneur.dev';
  const title = options.title || 'Official Launch Campaign Link';
  const tags = options.tags || 'launch,solopreneur,marketing';

  const taggedTarget = buildUtmUrl(destination, options.utm || {});

  info(`Target URL with UTM tags: ${taggedTarget}`);

  const createResult = await mcpClient.callTool('create_shortlink', {
    destination: taggedTarget,
    slug,
    host,
    title,
    tags,
  });

  const shortUrl = createResult.shortUrl || `https://${host}/${slug}`;
  success(`Branded Shortlink Created: ${shortUrl} -> ${taggedTarget}`);

  // Verification step: verify link presence via list_links tool
  const linksList = await mcpClient.callTool('list_links', { host });
  const found = (linksList || []).find((l) => l.slug === slug && l.host === host);

  return {
    slug,
    host,
    shortUrl,
    target: taggedTarget,
    verified: !!found,
  };
}
