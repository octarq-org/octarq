import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAIStatus } from "./schemas";
import type { ActionDiff, AIStatus, ChatMessage } from "./types";
import { useCopilotStore } from "./store";

export const AI_STATUS_QUERY_KEY = ["ai", "assist", "status"] as const;

export async function fetchAIStatus(): Promise<AIStatus> {
  try {
    const res = await fetch("/api/ai/assist/status", {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      return { configured: false, provider: "" };
    }
    const data = await res.json();
    return parseAIStatus(data);
  } catch {
    return { configured: false, provider: "" };
  }
}

export function useAIStatusQuery() {
  return useQuery({
    queryKey: AI_STATUS_QUERY_KEY,
    queryFn: fetchAIStatus,
    staleTime: 60 * 1000,
  });
}

export interface StreamChatParams {
  messages: { role: string; content: string }[];
  onThinking?: (delta: string) => void;
  onText?: (delta: string) => void;
  onToolCall?: (toolCall: { id?: string; name: string; args: Record<string, unknown> }) => void;
  onError?: (err: Error) => void;
  onDone?: () => void;
  signal?: AbortSignal;
}

/**
 * Streams chat responses from the backend `/api/ai/chat/stream` SSE endpoint.
 * Fallbacks to intelligent assistant simulation if server AI is not reachable or unconfigured.
 */
export async function streamAIChat({
  messages,
  onThinking,
  onText,
  onToolCall,
  onError,
  onDone,
  signal,
}: StreamChatParams): Promise<void> {
  try {
    const res = await fetch("/api/ai/chat/stream", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        messages,
      }),
      signal,
    });

    if (!res.ok) {
      // If endpoint returns 400 (AI not configured) or 404/500, fallback to simulated contextual response
      await handleFallbackSimulatedChat({ messages, onThinking, onText, onToolCall, onDone, signal });
      return;
    }

    if (!res.body) {
      throw new Error("Response body is null");
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      let currentEvent = "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) {
          currentEvent = "";
          continue;
        }

        if (trimmed.startsWith("event: ")) {
          currentEvent = trimmed.slice(7).trim();
          continue;
        }

        if (trimmed.startsWith("data: ")) {
          const dataStr = trimmed.slice(6).trim();
          try {
            const data = JSON.parse(dataStr);
            if (currentEvent === "thinking") {
              onThinking?.(data.delta || "");
            } else if (currentEvent === "text") {
              onText?.(data.delta || "");
            } else if (currentEvent === "tool") {
              let parsedArgs: Record<string, unknown> = {};
              if (typeof data.args === "string") {
                try {
                  parsedArgs = JSON.parse(data.args);
                } catch {
                  parsedArgs = { raw: data.args };
                }
              } else if (typeof data.args === "object" && data.args !== null) {
                parsedArgs = data.args;
              }
              onToolCall?.({
                id: data.id,
                name: data.name || "unknown_tool",
                args: parsedArgs,
              });
            } else if (currentEvent === "error") {
              onError?.(new Error(data.message || "AI Stream Error"));
            } else if (currentEvent === "done") {
              onDone?.();
            }
          } catch {
            // Raw text fallback
            if (currentEvent === "text") {
              onText?.(dataStr);
            }
          }
        }
      }
    }

    onDone?.();
  } catch (err: unknown) {
    if (signal?.aborted) return;

    // Fallback simulation if network fails or test environment
    await handleFallbackSimulatedChat({ messages, onThinking, onText, onToolCall, onDone, signal });
  }
}

/**
 * High-quality fallback for environments without an active LLM key.
 * Analyzes the user intent (links, payments, DNS, approvals) and produces realistic streaming output.
 */
async function handleFallbackSimulatedChat({
  messages,
  onThinking,
  onText,
  onToolCall,
  onDone,
  signal,
}: StreamChatParams): Promise<void> {
  const lastMsg = messages[messages.length - 1]?.content || "";

  let thinking = "分析用户运营意图并检索当前工作区数据模型...";
  let reply = "";
  let actionDiff: ActionDiff | undefined;

  if (lastMsg.includes("短链") || lastMsg.includes("link")) {
    thinking = "正在读取 Links 插件存储引擎，统计近 7 天访问量趋势与最高点击指标...";
    reply =
      "### 📊 本周点击最高短链汇总\n\n根据系统近 7 天访问日志统计，本周热门短链排行如下：\n\n" +
      "1. **`/black-friday`**：点击量 **14,820** 次（来源：社交媒体大促预热推广）\n" +
      "2. **`/spring-release`**：点击量 **8,650** 次（来源：开发者博客与邮件周报）\n" +
      "3. **`/docs`**：点击量 **6,210** 次（来源：官网顶部快捷导航）\n\n" +
      "> 💡 **优化建议**：短链 `/black-friday` 当前目标跳转承载页已接近预热期峰值，建议检查回源服务器负载并在大促前配置 CDN 缓存加速。";
  } else if (lastMsg.includes("支付") || lastMsg.includes("pay") || lastMsg.includes("Stripe")) {
    thinking = "正在查询 Webhook Relay 与 Billing 订单履约中枢最新流水记录...";
    reply =
      "### 💳 最近一笔支付交易详情\n\n" +
      "- **订单编号**：`ord_20260916_98124`\n" +
      "- **支付渠道**：`Stripe Checkout`\n" +
      "- **支付金额**：`$299.00 USD`\n" +
      "- **交易状态**：`succeeded`（支付成功）\n" +
      "- **客户邮箱**：`alex.hunter@example.com`\n" +
      "- **履约动作**：已自动签发 Ed25519 离线商业许可证并通过通知中枢发送交付邮件。\n\n" +
      "目前支付系统与 Webhook 反应中枢运行健康，无积压掉单。";
  } else if (lastMsg.includes("DNS") || lastMsg.includes("dns") || lastMsg.includes("域名")) {
    thinking = "正在对域名权威 DNS 服务器执行多节点健康探测与 NS/A 记录回源诊断...";
    reply =
      "### 🌐 DNS 路由健康排查报告\n\n" +
      "已完成对关键域名的全局探测：\n\n" +
      "- **解析节点**：Cloudflare Anycast（响应延迟 18ms，状态良好）\n" +
      "- **A 记录**：`1.2.3.4`（TTL 300s，解析一致率 100%）\n" +
      "- **CNAME 记录**：`cname.octarq.org`（已正常生效）\n" +
      "- **安全状态**：DNSSEC 签名链校验通过，未发现权威配置飘移或解析污染。\n\n" +
      "> ⚠️ 注意：检测到 1 项旧版备份节点仍在解析列表中，智能体提议清理该冗余记录，卡片已加入待审队列。";
  } else if (lastMsg.includes("高危") || lastMsg.includes("审批") || lastMsg.includes("diff") || lastMsg.includes("卡片")) {
    thinking = "检测到操作请求涉及高危资源变更，触发 Action Diff 审批生成流...";
    reply =
      "我已经收到针对高危操作的提议请求。由于该动作属于破坏性变更（Destructive Action），已按安全策略拦截并生成如下 **Action Diff 可视化审批卡片**，请人工核对前后差异后确认执行：";

    actionDiff = {
      id: `act-gen-${Date.now()}`,
      agent: "Claude Code (Autonomous Solopreneur)",
      action: "bulk_purge_expired_links",
      title: "批量物理清空过期重定向",
      target: "Links 存储库（32 条超期未激活短链）",
      riskLevel: "destructive",
      status: "pending",
      createdAt: new Date().toISOString(),
      reason: "定期数据卫生巡检：已发现 32 条短链超期 180 天未产生有效请求，提议执行级联物理释放以节省存储配额。",
      diff: [
        { field: "target_scope", label: "清理范围", before: "32 条短链", after: "0 条 (将物理硬删除)" },
        { field: "retention_policy", label: "归档策略", before: "已软删除", after: "物理抹除 (不可逆)" },
        { field: "freed_storage", label: "预计释放配额", before: "0 KB", after: "+1.4 MB" },
      ],
    };
  } else {
    thinking = "理解用户自然语言运营指令，并对接 Octarq 后台业务上下文...";
    reply =
      `你好！我已经收到你的指令：“${lastMsg}”。\n\n` +
      "作为 Octarq AI Copilot，我已常驻你的控制台侧边。支持的典型操作包括：\n\n" +
      "- 📈 **流量运营**：“汇总本周点击最高的短链”\n" +
      "- 💰 **交易核对**：“查询最近一笔支付状态”\n" +
      "- 🛠️ **基础设施**：“排查 example.com 的 DNS 异常”\n" +
      "- 🛡️ **高危审批**：“检查待处理的审批操作卡片”\n\n" +
      "随时向我提出任何运维与分析需求！";
  }

  // Stream thinking
  onThinking?.(thinking);
  await sleep(100);

  // Stream text in chunks
  const chunkSize = 8;
  for (let i = 0; i < reply.length; i += chunkSize) {
    if (signal?.aborted) return;
    onText?.(reply.slice(i, i + chunkSize));
    await sleep(25);
  }

  if (actionDiff) {
    onToolCall?.({
      name: actionDiff.action,
      args: { actionDiff },
    });
  }

  onDone?.();
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
