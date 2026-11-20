export default {
  async email(message, env, ctx) {
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

    const res = await fetch(env.OCTARQ_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "message/rfc822",
        "X-Octarq-To": message.to,
        "X-Octarq-From": message.from,
      },
      body,
    });

    if (!res.ok) {
      message.setReject(`octarq ingest failed: ${res.status}`);
    }
  },
};
