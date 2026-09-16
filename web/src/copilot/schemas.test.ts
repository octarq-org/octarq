import { describe, it, expect } from "vitest";
import {
  parseAIStatus,
  parseActionDiff,
  parseActionDiffList,
  actionDiffSchema,
} from "./schemas";
import type { ActionDiff } from "./types";

describe("copilot schemas", () => {
  it("parses valid AIStatus correctly", () => {
    const data = { configured: true, provider: "DeepSeek V3" };
    const parsed = parseAIStatus(data);
    expect(parsed.configured).toBe(true);
    expect(parsed.provider).toBe("DeepSeek V3");
  });

  it("handles malformed AIStatus gracefully using fallback", () => {
    const parsed = parseAIStatus(null);
    expect(parsed.configured).toBe(false);
    expect(parsed.provider).toBe("");

    const parsedBad = parseAIStatus({ configured: "not_a_bool" });
    expect(parsedBad.configured).toBe(false);
  });

  it("parses valid ActionDiff correctly", () => {
    const raw: ActionDiff = {
      id: "act-101",
      agent: "Claude Code",
      action: "delete_dns_record",
      title: "删除 DNS 解析",
      target: "api.octarq.com",
      riskLevel: "destructive",
      status: "pending",
      createdAt: "2026-09-16T10:00:00Z",
      diff: [
        { field: "domain", label: "域名", before: "api.octarq.com", after: "api.octarq.com" },
        { field: "value", label: "解析值", before: "1.2.3.4", after: "(已删除)" },
      ],
    };

    const parsed = parseActionDiff(raw);
    expect(parsed.id).toBe("act-101");
    expect(parsed.riskLevel).toBe("destructive");
    expect(parsed.status).toBe("pending");
    expect(parsed.diff).toHaveLength(2);
  });

  it("falls back safely on malformed ActionDiff without throwing", () => {
    const parsed = parseActionDiff({ invalid_field: 123 });
    expect(parsed.id).toBe("unknown");
    expect(parsed.status).toBe("pending");
    expect(parsed.riskLevel).toBe("write");
  });

  it("parses ActionDiff list with filter/fallback", () => {
    const list = [
      {
        id: "act-1",
        agent: "Agent A",
        action: "update",
        title: "Test",
        target: "res-1",
        riskLevel: "write",
        status: "pending",
        createdAt: "2026-09-16T10:00:00Z",
        diff: [],
      },
    ];
    const parsed = parseActionDiffList(list);
    expect(parsed).toHaveLength(1);
    expect(parsed[0].id).toBe("act-1");

    expect(parseActionDiffList("invalid")).toEqual([]);
  });

  it("validates riskLevel enum values strictly", () => {
    const validLevels = ["read", "write", "destructive"];
    for (const level of validLevels) {
      const res = actionDiffSchema.safeParse({
        id: "id",
        agent: "agent",
        action: "act",
        title: "t",
        target: "target",
        riskLevel: level,
        status: "pending",
        createdAt: "2026-09-16T10:00:00Z",
        diff: [],
      });
      expect(res.success).toBe(true);
    }

    const invalidRes = actionDiffSchema.safeParse({
      id: "id",
      agent: "agent",
      action: "act",
      title: "t",
      target: "target",
      riskLevel: "super_dangerous",
      status: "pending",
      createdAt: "2026-09-16T10:00:00Z",
      diff: [],
    });
    expect(invalidRes.success).toBe(false);
  });
});
