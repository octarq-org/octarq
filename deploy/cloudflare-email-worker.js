/**
 * Cloudflare Email Worker — forwards inbound mail to your led instance.
 *
 * Setup:
 *  1. Cloudflare Dashboard → your domain → Email → Email Routing → enable.
 *  2. Workers & Pages → create a Worker, paste this file.
 *  3. Set one variable (Settings → Variables):
 *       LED_ENDPOINT = your Inbound Webhook URL, copied from led
 *                      Settings → Mailboxes. It already contains your org slug
 *                      and secret token in the path, e.g.
 *                      https://your-led-host/api/webhook/<orgSlug>/email/inbound/<token>
 *  4. Email Routing → Routes → set a catch-all (or per-address) action to
 *     "Send to a Worker" → this Worker.
 *
 * led parses the raw RFC822 message, matches the recipient to a mailbox
 * (or catch-all creates one), and stores it.
 */
export default {
  async email(message, env, ctx) {
    // Read the raw message stream into a single ArrayBuffer.
    const reader = message.raw.getReader();
    const chunks = [];
    let total = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      total += value.length;
    }
    const body = new Uint8Array(total);
    let off = 0;
    for (const c of chunks) {
      body.set(c, off);
      off += c.length;
    }

    const res = await fetch(env.LED_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "message/rfc822",
        "X-Led-To": message.to,
        "X-Led-From": message.from,
      },
      body,
    });

    if (!res.ok) {
      // Returning an error lets Cloudflare retry / report delivery failure.
      message.setReject(`led ingest failed: ${res.status}`);
    }
  },
};
