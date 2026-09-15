// MCP Client implementation for Octarq
// Supports live connection to Octarq's Remote MCP endpoint (/api/mcp/sse or /api/mcp/stream)
// and seamless fallback to Simulation Mode for zero-friction standalone developer evaluation.

import { info, warn, success, mcpCall, mcpResult } from './utils.mjs';

export class OctarqMCPClient {
  /**
   * @param {object} options
   * @param {string} [options.baseUrl]
   * @param {string} [options.token]
   * @param {string} [options.mode] 'auto' | 'live' | 'simulate'
   */
  constructor(options = {}) {
    this.baseUrl = (options.baseUrl || process.env.OCTARQ_URL || 'http://localhost:8080').replace(/\/+$/, '');
    this.token = options.token || process.env.OCTARQ_TOKEN || '';
    this.mode = options.mode || process.env.EXECUTION_MODE || 'auto';
    this.isLive = false;

    // Simulated in-memory database state
    this.simulatedState = {
      domains: [
        {
          id: 1,
          name: process.env.SOLOPRENEUR_DOMAIN || 'solopreneur.dev',
          forMail: true,
          forLink: true,
          zoneId: 'zone_solopreneur_9841',
        },
      ],
      links: [
        {
          id: 101,
          host: process.env.SOLOPRENEUR_SHORTLINK_HOST || 'go.solopreneur.dev',
          slug: 'github',
          target: 'https://github.com/octarq-org/octarq',
          title: 'Octarq Open Source Core',
          tags: 'oss,core,git',
          clicks: 342,
          enabled: true,
          archived: false,
          createdAt: new Date(Date.now() - 86400000 * 5).toISOString(),
        },
      ],
      mailboxes: [
        {
          id: 1,
          address: process.env.SOLOPRENEUR_MAILBOX || 'ops@solopreneur.dev',
          enabled: true,
          unread: 1,
        },
      ],
      emails: [
        {
          id: 901,
          mailboxId: 1,
          from: 'accounts@auth.stripe.com',
          to: process.env.SOLOPRENEUR_MAILBOX || 'ops@solopreneur.dev',
          subject: 'Stripe Security: Verification Code is 849201',
          snippet: 'Use verification code 849201 to complete your payout account setup. This code expires in 10 minutes.',
          read: false,
          receivedAt: new Date(Date.now() - 60000 * 2).toISOString(),
        },
      ],
    };
  }

  /**
   * Initializes client and negotiates live connection or simulation mode.
   */
  async init() {
    if (this.mode === 'simulate') {
      info('Running in Forced Simulation Mode (Mock Octarq MCP Server).');
      this.isLive = false;
      return;
    }

    try {
      const probeUrl = `${this.baseUrl}/api/mcp/sse${this.token ? `?token=${encodeURIComponent(this.token)}` : ''}`;
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 1200);

      const res = await fetch(probeUrl, {
        method: 'GET',
        headers: {
          Accept: 'text/event-stream',
          ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
        },
        signal: controller.signal,
      }).catch(() => null);

      clearTimeout(timeout);

      if (res && (res.status === 200 || res.status === 400 || res.status === 401)) {
        this.isLive = true;
        success(`Connected to live Octarq instance at ${this.baseUrl}`);
      } else {
        if (this.mode === 'live') {
          throw new Error(`Unable to connect to Octarq live instance at ${this.baseUrl} (Status: ${res ? res.status : 'offline'})`);
        }
        info(`Octarq live instance not detected at ${this.baseUrl}. Activating Autonomous Simulation Mode.`);
        this.isLive = false;
      }
    } catch (err) {
      if (this.mode === 'live') throw err;
      info(`Live probe failed (${err.message}). Activating Autonomous Simulation Mode.`);
      this.isLive = false;
    }
  }

  /**
   * Universal MCP Tool Invocation
   * @param {string} toolName
   * @param {object} args
   */
  async callTool(toolName, args = {}) {
    mcpCall(toolName, args);

    if (this.isLive) {
      return this._callLiveMcp(toolName, args);
    }
    return this._callSimulatedMcp(toolName, args);
  }

  async _callLiveMcp(toolName, args) {
    // If live Octarq is running, execute tool via REST dual endpoint or MCP
    try {
      if (toolName === 'list_domains') {
        const res = await fetch(`${this.baseUrl}/api/domains`, {
          headers: { Authorization: `Bearer ${this.token}` },
        });
        const data = await res.json();
        mcpResult(toolName, data);
        return data;
      }

      if (toolName === 'create_shortlink') {
        const res = await fetch(`${this.baseUrl}/api/links/declarative`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${this.token}`,
          },
          body: JSON.stringify(args),
        });
        const data = await res.json();
        mcpResult(toolName, data);
        return data;
      }

      if (toolName === 'list_links') {
        const res = await fetch(`${this.baseUrl}/api/links`, {
          headers: { Authorization: `Bearer ${this.token}` },
        });
        const data = await res.json();
        mcpResult(toolName, data);
        return data;
      }

      if (toolName === 'list_mailboxes') {
        const res = await fetch(`${this.baseUrl}/api/mailboxes`, {
          headers: { Authorization: `Bearer ${this.token}` },
        });
        const data = await res.json();
        mcpResult(toolName, data);
        return data;
      }

      if (toolName === 'list_emails') {
        const query = args.mailboxId ? `?mailboxId=${args.mailboxId}` : '';
        const res = await fetch(`${this.baseUrl}/api/emails${query}`, {
          headers: { Authorization: `Bearer ${this.token}` },
        });
        const data = await res.json();
        mcpResult(toolName, data);
        return data;
      }
    } catch (err) {
      warn(`Live tool invocation failed (${err.message}), falling back to simulation`);
    }

    return this._callSimulatedMcp(toolName, args);
  }

  async _callSimulatedMcp(toolName, args) {
    switch (toolName) {
      case 'list_domains': {
        const result = this.simulatedState.domains;
        mcpResult(toolName, result);
        return result;
      }

      case 'create_shortlink': {
        const newLink = {
          id: this.simulatedState.links.length + 101,
          host: args.host || 'go.solopreneur.dev',
          slug: args.slug || `s${Math.random().toString(36).substring(2, 7)}`,
          target: args.destination,
          title: args.title || 'Campaign Short Link',
          tags: args.tags || 'marketing,agent',
          clicks: 0,
          enabled: true,
          archived: false,
          createdAt: new Date().toISOString(),
          shortUrl: `https://${args.host || 'go.solopreneur.dev'}/${args.slug}`,
        };
        this.simulatedState.links.unshift(newLink);
        const result = {
          success: true,
          link: newLink,
          shortUrl: newLink.shortUrl,
        };
        mcpResult(toolName, result);
        return result;
      }

      case 'list_links': {
        let result = this.simulatedState.links;
        if (args.host) {
          result = result.filter((l) => l.host === args.host);
        }
        mcpResult(toolName, result);
        return result;
      }

      case 'list_mailboxes': {
        const result = this.simulatedState.mailboxes;
        mcpResult(toolName, result);
        return result;
      }

      case 'list_emails': {
        let result = this.simulatedState.emails;
        if (args.mailboxId) {
          result = result.filter((e) => e.mailboxId === args.mailboxId);
        }
        if (args.unreadOnly) {
          result = result.filter((e) => !e.read);
        }
        mcpResult(toolName, result);
        return result;
      }

      default:
        throw new Error(`Unknown MCP tool: ${toolName}`);
    }
  }

  /**
   * Helper to simulate a new incoming verification email
   */
  injectSimulatedEmail({ from, subject, snippet }) {
    const newEmail = {
      id: this.simulatedState.emails.length + 901,
      mailboxId: 1,
      from: from || 'noreply@saas-provider.com',
      to: process.env.SOLOPRENEUR_MAILBOX || 'ops@solopreneur.dev',
      subject: subject || 'Verify your new account',
      snippet: snippet || 'Your one-time passcode is 318492. Enter this code to verify your identity.',
      read: false,
      receivedAt: new Date().toISOString(),
    };
    this.simulatedState.emails.unshift(newEmail);
    return newEmail;
  }
}
