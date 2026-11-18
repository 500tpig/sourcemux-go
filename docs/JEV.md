# JEV 模式与质量判断

JEV 在 SourceMux 中用于两类可选场景：
1. 研究模式（`mode=jev`）：选择研究动作和判断已读资料是否足够。搜索和抓取继续使用已有渠道，最终答案由独立的 reasoning 模型生成。默认 `mode=fixed`。
2. 普通搜索语义质量检查（`answer_check`，默认关闭）：对普通 `search` / `web_search` 返回的 Grok 模型回答检查是否实质回答问题，以及可见引用来源的标题或摘要是否直接反驳主结论。

两者均不默认给每次普通搜索增加模型调用；只有显式启用时才会请求判断服务。

## 配置与调用

在唯一的运行配置文件中增加顶层 `jev`，v1、v2 均支持：

```json
{
  "jev": {
    "apiURL": "https://api.typesafe.ai/v1",
    "apiKey": "YOUR_TYPESAFE_API_KEY",
    "model": "jev-latest",
    "timeoutSec": 15,
    "answerCheck": false
  }
}
```

这是需合入现有文件的配置片段；搜索和抓取仍需已有渠道配置。`apiURL`、`model`、`timeoutSec`、`answerCheck` 均可省略并使用示例中的默认值，超时范围为 1–300 秒。地址可包含 `/systemone`，客户端不会重复追加。`config migrate` 保留 JEV 配置；诊断输出只显示 Key 是否配置及 `answerCheck` 状态。

### 研究模式调用

```bash
sourcemux --config ./sourcemux.json research "比较两个项目的维护现状" --mode jev --depth standard --max-fetches 3 --json
sourcemux --config ./sourcemux.json smart-answer "比较两个项目的维护现状" --mode jev --depth standard --json
```

MCP 的 `research_run` 和 `smart_answer` 均支持以下参数；其他参数沿用原工具：

```json
{"query":"比较两个项目的维护现状","mode":"jev","depth":"standard","max_fetches":3}
```

### 普通搜索回答语义检查（可选）

普通搜索默认不开启检查（`jev.answerCheck=false`）。可通过配置文件开启，也可在单次调用时显式覆盖（包括用 `false` 临时关闭）：

```bash
sourcemux --config ./sourcemux.json search "最新规范变更" --answer-check --json
sourcemux --config ./sourcemux.json search "ping" --answer-check=false --json
```

MCP `web_search` 支持同名可选布尔参数 `answer_check`（省略时继承 `jev.answerCheck`，传入 `true` 或 `false` 覆盖本次调用）：

```json
{"query":"最新规范变更","answer_check":true}
```

触发与预算规则：
- **仅作用于普通直接 search 的 `grok-pool` 回答**：`research`（`fixed`/`jev`）和 `smart-answer` 内部调用搜索时跳过此检查；TinyFish、Exa、Tavily 等源检索结果也不再单独调用 JEV。
- **空内容或无有效 HTTP(S) URL 先走既有本地校验**：在进入 JEV 之前直接按 `no_content` / `no_valid_sources` 触发 fallback，不浪费 JEV 调用。
- **每次搜索最多一次逻辑检查**：底层复用 `JEVClient.Evaluate`（遇到 HTTP 429/529 最多尝试 3 次，因此底层并非严格只有一次 HTTP 请求），单次检查受 `jev.timeoutSec`（默认 15 秒）与调用方总截止时间双重约束。
- **拒绝、降级与取消契约**：
  - `substantive < 0.5`（未实质回答，如仅说准备搜索）或 `contradicted >= 0.5`（主要结论被引用标题/摘要直接反驳）时，将 Grok 尝试标记为 `answer_rejected` 并进入既有 fallback（TinyFish -> Exa -> Tavily）；传 `--no-fallback` / `no_fallback=true` 时不会绕过拒绝，而是直接返回失败并在诊断信息中保留拒绝原因。
  - 若 JEV 判断失败（如网络/HTTP 错误、非法概率或 JEV 单次超时）或未配置 `jev.apiKey`：不丢弃原搜索结果，保留 Grok 回答并标记 `answer_check.status="unverified"`（`reason` 为 `jev_error` 或 `jev_unconfigured`）；`--agent` 输出会额外写入告警，避免把未验证结果冒充为已验证。
  - 若是调用方主动取消或外层总截止时间到期，遵循原有取消/超时契约直接失败，不会当作普通 JEV 故障恢复成成功。
- **结果字段与文案语义**：
  - JSON 新增可选 `answer_check`（未启用时省略），包含 `status`（`passed` / `rejected` / `unverified`）、`reason`、`scores`、`elapsed_ms`、上游实际返回的 `input_tokens` / `output_tokens` 及脱敏 `error`。
  - **`status="rejected"` 描述的是 Grok 原回答被拒**：即使后续 fallback 成功返回了 TinyFish/Exa/Tavily 结果，文本中的 `grok_answer_check: rejected (...; switched to fallback ...)` 和 JSON 中的 `answer_check` 仍表示 Grok 原回答被拒，不代表最终返回的 fallback 结果被拒。
- **证据边界**：仅透传 Grok 上游实际返回的来源标题和摘要元数据（未返回时留空，不猜字段也不额外抓取网页）。标题和摘要检查只能发现明显矛盾，**不能称为全文事实核验**。

## 执行和预算

每轮提供已配置搜索渠道、计划查询、固定缺口查询以及最多五个已发现 URL。每个候选动作附带基于当前实现能力的 `guidance`（例如说明搜索操作不传服务端域名/时间过滤、摘要不等于已读正文、`web_fetch` 仅适合读取已知 URL 且受允许域名与抓取次数约束）。Grok 候选还受所选 profile 限制。选择概率至少 0.5 才执行；相同动作不重复，指定搜索渠道不在该动作内切换供应商。后续轮次可以选择其他渠道；抓取仍使用现有兜底路由。每轮动作顺序执行，以便直接共享取消与预算，不额外并发。

| 深度 | 最多轮数 | 每轮最多搜索／抓取动作 | 最多阅读页数 | 总时限 |
| --- | --- | --- | --- | --- |
| quick | 2 | 1 | 2 | 45 秒 |
| standard | 3 | 2 | 6 | 120 秒 |
| deep | 5 | 3 | 15 | 300 秒 |

搜索／抓取动作总数不超过轮数乘每轮上限。`max_fetches` 进一步限制全程抓取数；后续分页从内存读取，不重新请求网页、不重复计抓取次数。JEV 模式不自动执行 map/crawl。较短的调用方截止时间优先，JEV 重试和退避也计入预算。smart-answer 的研究和综合共用总截止时间，研究用尽时间后综合可能无法执行。

抓取到非空正文后评估有用性、充分性和缺口。有用性至少 0.5、充分性至少 0.75，且存在已读正文时才提前结束。渠道重试次数、返回条数沿用既有适配器；动作上限不是底层 HTTP 请求数量上限。

## 分页阅读与累计摘录

每个来源先在任务内保留最多 75,000 字符正文。首次读取约 5,000 字符，优先在段落边界结束；后续页面与前一页重叠约 200 字符，避免遗漏边界内容。

每次读取后，从该来源已读范围内选择相关原文片段，累计不超过 2,000 字符，包含片段间分隔符。选择依据是问题中的词语及中文双字匹配，保留原文位置，不生成新句子或额外调用综合模型。它不保证识别同义表达，也可能省略必要背景。

同轮抓取的来源先各读取首段，再由 JEV 评估累计摘录。证据仍不足时，按来源顺序继续读取后文，每翻一页重新评估一次。充分则停止；判断服务失败也停止，不把错误当作翻页信号。读完整篇仍不足时，可在剩余研究预算内寻找其他来源。首段计入全程阅读页数，达到上限以 `reading_limit` 停止。

JEV 与最终综合使用同一份证据摘录。综合输入不再包含搜索摘要、未读候选链接或启发式 `confirmed_facts`，这些发现信息仍保留在完整研究包中。

## 输出与失败处理

CLI `--json` 的研究包增加 `jev`：

| 字段 | 含义 |
| --- | --- |
| `assessment` | `unknown`、`empty`、`insufficient` 或 `sufficient`，以及概率和缺口 |
| `citations` | 抓取成功、正文非空且通过域名限制的 URL |
| `rounds` | 本轮选择的动作及已知候选分数 |
| `calls` | 判断阶段、耗时、可用的 token 用量和安全错误 |
| `read_pages`、`read_page_limit` | 全部来源累计读取页数及上限 |
| `stop_reason` | 停止原因 |
| `warnings` | 判断失败等说明 |

常见停止原因包括 `sufficient`、`round_limit`、`reading_limit`、`deadline`、`actions_exhausted`、`no_suitable_actions`、`no_new_evidence` 和 `judgment_unavailable`。连续两轮没有新增来源或正文时，以 `no_new_evidence` 结束。低于阈值时不会强行选择最高分动作。

`fetched_pages_summary[].reading` 记录各来源的 `content_chars`、`read_chars`、`pages_read`、`complete`、`truncated` 和 `passages`。`complete` 表示本地已扫描完整正文，不表示模型逐字检查过全文。`truncated` 表示正文超过 75,000 字符的保留上限；模型看到的摘录视图另有 `truncated` 标记，表示摘录省略了原文。每个 passage 的 `start`、`end` 是原文 Unicode 字符的从零开始、右端不包含的区间，`text` 是对应原文。

缺少 Key 或无可用搜索渠道时返回错误。运行中判断服务失败则保留证据，标记 `unknown` 并停止；不会静默重跑固定流程。没有正文时为 `empty`，搜索摘要不能使状态变为充分。普通文本和 MCP 文本显示状态、停止原因与已读引用；`--agent` 保留未知或不足状态。

smart-answer 提示词仅允许引用已读 URL，并传入判断状态。综合模型仍可能违反提示，需要对重要结论核查引用；程序不将概率当作事实正确率。综合失败时，CLI JSON 和 MCP 错误文本保留已有证据。

## 数据与验证边界

问题、候选查询、URL、动作历史和正文片段会发送给配置的 JEV 服务。供应商凭据不放入判断 state 或 trace；Key 仅用于认证。网页内容作为待分析数据，不能增加执行动作或改变预算。

JEV 判断和综合使用的证据预览仍限制为约 20 KB，并标记截断与省略；模型只检查实际发送的摘录。分页会增加判断调用次数，费用和延迟随阅读页数变化。协议参考 [smartsearch 固定提交中的客户端](https://github.com/konbakuyomu/smartsearch/blob/1969e4fa32a74d0d48b2c752afbadf496e71ef6e/src/smart_search/jev.py)；本地测试不证明真实服务当前可用性、费用或判断质量。

分页仅用于 `mode=jev`。fixed 模式仍保留每页 6,000 字符预览及原有 48 KB 综合输入上限。MCP 精简展示长度保持原值，不影响分页后的模型证据输入。
