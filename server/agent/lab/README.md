# Offline Agent Harness Lab (`server/agent/lab`)

## 1. 概述与设计理念

`server/agent/lab` 是 Octarq 智能体运行时（Agent Harness）的**离线实验台**。
它借鉴了 [google-research/harness2](https://github.com/google-research/harness2) 的 **Propose–Probe–Compose** 三段式结构，通过回放式测试与确定性打分，产出供人工审阅采纳的评估报告：

```
+-----------+      +-------------------+      +-------------------------+
|  Propose  | ---> |       Probe       | ---> |         Compose         |
| 候选 Specs |      | 任务集确定性回放  |      | 确定性评分 + 差异分析报告|
+-----------+      +-------------------+      +-------------------------+
```

- **Propose（提议）**：由工程师或离线流程提出若干 `harness.HarnessSpec` 候选文件；
- **Probe（探针）**：离线驱动 `harness.Runner`，消费脚本化模型回复与预设工具输出，采集 `harness.TraceEvent` 轨迹证据；
- **Compose（综合）**：按严格确定性的评价准则进行排序，输出 Markdown 摘要与完整 JSON 报告，高亮「基线 vs 最佳候选」的逐字段差异。

### 明确不做的事项（Explicit Non-Goals）

- **禁止在线自修改**：严禁在生产运行时动态修改线上提示词或运行时配置。
- **禁止在线请求路径探测**：严禁在 HTTP/MCP 实时请求链路上插入探针或分流。
- **禁止多轮自主 LLM 提案**：候选生成完全位于离线工程流程中。
- **禁止全自动采纳**：Lab **只读文件、只写报告目录**，绝不连接生产数据库，绝不修改配置。**任何采纳决策必须由人类操作员（Human-in-the-Loop）审阅确认后手动执行。**

---

## 2. 格式规范

### 2.1 任务集格式（`tasks.json`）

任务集文件由版本号、工具目录元数据以及具体的任务用例列表组成：

```json
{
  "version": 1,
  "name": "example-ops-tasks",
  "description": "Deterministic operational tasks for Octarq assistant evaluation",
  "catalog": [
    {
      "name": "lookup_domain",
      "summary": "Check domain registration status",
      "method": "GET",
      "path": "/api/domains/{domain}",
      "parameters": [
        {
          "name": "domain",
          "type": "string",
          "required": true,
          "description": "Domain name to lookup"
        }
      ]
    }
  ],
  "tasks": [
    {
      "id": "check-domain-status",
      "title": "Lookup active domain registration",
      "input": "Check if octarq.org is registered and active",
      "mock_responses": [
        "```tool_call\n{\"name\": \"lookup_domain\", \"args\": {\"domain\": \"octarq.org\"}}\n```",
        "The domain octarq.org is active and registered."
      ],
      "mock_tools": {
        "lookup_domain": {
          "output": "{\"domain\":\"octarq.org\",\"status\":\"active\"}"
        }
      },
      "criteria": {
        "expected_status": "done",
        "expected_tools": ["lookup_domain"],
        "forbidden_tools": ["delete_domain"],
        "final_contains": "octarq.org is active",
        "max_steps": 2,
        "max_tool_errors": 0
      }
    }
  ]
}
```

#### 判据说明（`criteria`）
- `expected_status`：期望的最终 Turn 状态（如 `"done"`）；
- `expected_tools`：必须调用的工具名称列表；
- `forbidden_tools`：严禁调用的工具名称列表；
- `final_contains`：助手最终输出必须包含的子串；
- `max_steps`：最大允许执行的工具步数；
- `max_tool_errors`：最大允许的工具错误数（通常为 `0`）。

### 2.2 候选定义格式（`candidate.json`）

每个候选直接消费核心已有的 `harness.HarnessSpec` 数据结构，拒绝未知字段：

```json
{
  "version": 1,
  "name": "candidate-optimized",
  "preamble": "Octarq ops assistant.",
  "extraRules": [
    "Verify parameters before execution."
  ],
  "toolAllow": [
    "create_link",
    "lookup_domain"
  ],
  "maxSteps": 4
}
```

---

## 3. 命令行用法（CLI）

编译并运行 `cmd/agent-lab`：

```bash
agent-lab -tasks <task-set.json> -candidates <dir-or-file>... -out <report-dir> [-baseline <name>] [-propose <proposal.json>]
```

### 示例

```bash
cd /Volumes/PHD/code/.worktrees/harness-lab/server
go run ./cmd/agent-lab \
  -tasks ./agent/lab/testdata/tasks.example.json \
  -candidates ./agent/lab/testdata/candidates \
  -out /tmp/agent-lab-report \
  -propose /tmp/proposal.json
```

### 退出码契约（Fail-Closed）
- `0`：成功生成 `report.json` 与 `report.md`（若提供 `-propose` 则同时生成 `proposal.json`）；
- `1`：参数缺失或格式错误；
- `2`：任务集文件不存在、无法读取或校验失败；
- `3`：候选文件解析失败、格式不合法或违反 `harness.Validate`；
- `4`：Lab 运行时回放故障；
- `5`：Proposal 构建校验失败或无推荐候选；
- `6`：Proposal 文件写入失败。

---

## 4. 确定性评分口径

评分过程**完全确定（Deterministic）**，严禁使用 `time.Now()`、`math/rand` 或网络请求：

1. **任务成功数（SuccessTasks）**：符合全部判定条件（`Criteria`）的任务总数（越高越优）；
2. **工具错误数（ToolErrors）**：工具调用抛出错误的累计次数（越低越优）；
3. **总步数（TotalSteps）**：完成任务耗费的步数（越低越优）；
4. **预估 Tokens（EstimatedTokens）**：基于字符长度严格换算（~4 字符/Token），综合 System Prompt、用户输入与对话历史（越低越优）；
5. **决胜排序（Tie-breaker）**：按候选名称字典序升序稳定决胜。
