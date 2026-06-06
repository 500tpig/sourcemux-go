# SourceMux 架构

这份文档说明 SourceMux 的运行结构和 provider 路由。图用 Mermaid 写在仓库里，代码改了，路由也能在同一个变更里对齐。

## 阅读入口

SourceMux 是一个 Go 单文件二进制程序，有两个运行入口：

- 给人、脚本和生成的 agent skill 使用的 CLI。
- 给 agent client 使用的 stdio MCP server。

两个入口共用同一套配置加载、engine client 和大部分路由逻辑。可以按这条线读代码：

入口 -> 表层适配 -> 共享 tool workflow -> router -> provider adapter -> engine client

```mermaid
flowchart LR
    human["人 / 脚本"]:::actor
    agent["AI agent / MCP client"]:::actor

    cli["sourcemux CLI"]:::entry
    mcp["stdio MCP server"]:::entry
    cfg["单个 JSON 配置文件"]:::config

    runtime["SourceMux 运行时"]:::core
    search["搜索 providers"]:::provider
    fetch["抓取 providers"]:::provider
    docs["文档 provider"]:::provider
    site["站点 map / crawl providers"]:::provider
    reason["推理 endpoints"]:::provider

    human --> cli
    agent --> mcp
    cli --> runtime
    mcp --> runtime
    cfg --> runtime
    runtime --> search
    runtime --> fetch
    runtime --> docs
    runtime --> site
    runtime --> reason

    classDef actor fill:#f7efe3,stroke:#8b5e2e,color:#2c2116;
    classDef entry fill:#e7f0ff,stroke:#3b70d6,color:#10233f;
    classDef config fill:#f3f4f6,stroke:#6b7280,color:#111827;
    classDef core fill:#e7f8ef,stroke:#2f8f5b,color:#0e3320;
    classDef provider fill:#fff4d6,stroke:#b7791f,color:#3a2503;
```

代码位置：

- 二进制入口：`cmd/sourcemux/main.go`
- CLI 和 MCP 的运行分流：`internal/app/app.go`
- CLI 分发：`internal/cli/cli.go`
- MCP server 组装：`internal/server/server.go`
- 配置加载：`internal/config/config.go`

## 运行层

CLI 和 MCP server 的输入、输出格式不同，但 search、fetch、research、synthesis 都会进入同一层共享 workflow。

```mermaid
flowchart TB
    subgraph Binary["二进制入口"]
        main["cmd/sourcemux/main.go"]
        app["internal/app.Run"]
    end

    subgraph Surfaces["运行入口"]
        cli["internal/cli/*\nflags, JSON, 人类可读文本"]
        server["internal/server/*\nMCP tool 注册"]
    end

    subgraph Shared["共享 workflow"]
        tools["internal/tools/*\nweb_search, web_fetch,\nresearch_run, smart_answer"]
        router["internal/router/*\n有序 fallback + route trace"]
        adapters["internal/router/adapters/*\nprovider 专用 adapter"]
    end

    subgraph Foundations["底层依赖"]
        config["internal/config/*\n一个选中的配置文件"]
        engine["internal/engine/*\nREST client, pool, retry"]
        capability["internal/capability/*\n共享 request/result contract"]
    end

    main --> app
    app --> cli
    app --> server
    cli --> tools
    server --> tools
    tools --> router
    router --> adapters
    adapters --> engine
    tools --> capability
    router --> capability
    app --> config
    cli --> config
    server --> config
    config --> engine

    classDef binary fill:#eef2ff,stroke:#4f46e5,color:#111827;
    classDef surface fill:#e0f2fe,stroke:#0284c7,color:#082f49;
    classDef shared fill:#dcfce7,stroke:#16a34a,color:#052e16;
    classDef foundation fill:#fef3c7,stroke:#d97706,color:#3f2a05;
    class main,app binary;
    class cli,server surface;
    class tools,router,adapters shared;
    class config,engine,capability foundation;
```

边界如下：

- `internal/cli/providers.go` 构造 `tools.WebSearchClients` 和 `tools.WebFetchClients`。MCP 侧也拿到同一类 client 结构。
- `internal/tools` 放共享行为。上层 workflow 调 `RunWebSearch` 和 `RunWebFetch`，不重新写 provider 顺序。
- `internal/router` 负责 fallback 执行和 route trace。provider 请求如何组装，放在 adapter 或 engine client 里。
- `internal/engine` 放可复用 REST client、endpoint pool、retry 和结果格式处理。

## 能力路由

顶层 CLI 命令和 MCP tool 的路由方式并不完全一样。有的走 fallback router，有的是直接 provider 调用。

| 能力 | CLI 入口 | MCP 入口 | 路由 |
| --- | --- | --- | --- |
| 网页搜索 | `sourcemux search` | `web_search` | Grok pool -> TinyFish Search -> Exa Search -> Tavily Search |
| 网页抓取 | `sourcemux fetch` | `web_fetch` | policy-first fetch：GitHub URL 优先 repo-aware；普通网页默认 Firecrawl -> Jina -> Exa -> Tavily -> TinyFish；cheap profile 为 Jina-first |
| 文档搜索 | `sourcemux docs-search` | `docs_search` | Exa docs/web search fallback |
| Exa 高级接口 | `exa-search`, `exa-contents` | `exa_search_advanced`, `exa_contents_advanced` | 直接调用 Exa |
| 站点 map | `sourcemux map` | `web_map` | 直接调用 Tavily Map |
| 站点 crawl | `sourcemux crawl` | `web_crawl` | 直接调用 Tavily Crawl |
| Firecrawl scrape | `firecrawl-scrape` | 无专用 tool | 直接调用 Firecrawl CLI command |
| Firecrawl map | `firecrawl-map` | 无专用 tool | 直接调用 Firecrawl CLI command |
| Research | `sourcemux research` | `research_run` | Plan -> search -> source cache -> rank -> fetch -> 可选 map/crawl |
| Smart answer | `sourcemux smart-answer` | `smart_answer` | Research -> reasoning endpoint synthesis |

Firecrawl 这里有两层含义：

- 现在没有专用 Firecrawl MCP tool，也没有接入 Firecrawl MCP server。
- Firecrawl 可以参与普通 `fetch --profile auto`。条件是当前配置启用 Firecrawl 且带有 key；顶层 `firecrawl` 配置即可启用默认 quality-first 参与，v2 `capabilities.web_fetch.providers` 用来显式覆盖 auto 顺序。`research_run` 使用共享 fetch workflow，所以 research 的页面抓取也会跟随同一策略。

## Search 流程

Search 面向 source。它返回精简 MCP text 和 `session_id`；agent 可以继续调用 `get_sources` 取缓存的 source URL。

```mermaid
flowchart LR
    req["query + platform + profile"]:::input
    policy["SearchPolicy\n解析 default / auto / heavy"]:::policy
    router["router.Run(MainSearch)"]:::core

    grok["Grok pool\n按 profile 过滤 endpoints"]:::provider
    tiny["TinyFish Search"]:::provider
    exa["Exa Search"]:::provider
    tavily["Tavily Search"]:::provider

    cache["source cache\nsession_id -> URLs"]:::cache
    result["WebSearchResult\ncontent + route_trace"]:::output

    req --> policy --> router
    router --> grok
    grok -- 空结果 / 错误 / 超时 --> tiny
    tiny -- 空结果 / 错误 --> exa
    exa -- 空结果 / 错误 --> tavily
    grok -- 成功 --> cache
    tiny -- 成功 --> cache
    exa -- 成功 --> cache
    tavily -- 成功 --> cache
    cache --> result

    classDef input fill:#f3f4f6,stroke:#6b7280,color:#111827;
    classDef policy fill:#ede9fe,stroke:#7c3aed,color:#2e1065;
    classDef core fill:#dcfce7,stroke:#16a34a,color:#052e16;
    classDef provider fill:#fff7ed,stroke:#ea580c,color:#431407;
    classDef cache fill:#e0f2fe,stroke:#0284c7,color:#082f49;
    classDef output fill:#ecfccb,stroke:#65a30d,color:#1a2e05;
```

代码位置：

- 共享 search workflow：`internal/tools/search.go`
- Search provider 顺序：`internal/tools/search.go` 里的 `searchProviders`
- Profile 解析：`internal/tools/profile_policy.go` 里的 `ResolveSearchProfile`
- MCP source cache：`internal/server/server.go` 里的 `server.App.CacheSources` 和 `server.App.GetSources`

## Fetch 流程

Fetch 顺序来自 profile + URL intent + 可选 v2 显式顺序。`auto` 默认是 policy-first / quality-first；GitHub URL 先走 repo-aware provider，普通网页优先 Firecrawl，`cheap` profile 才是 Jina-first。

```mermaid
flowchart TB
    config["config.LoadFile"]:::config

    subgraph V1["旧版配置"]
        v1["顶层 provider 配置块"]
        defaultOrder["policy 默认顺序：\ngithub/firecrawl, jina, exa, tavily, tinyfish"]
    end

    subgraph V2["Capabilities 配置"]
        v2["capabilities.web_fetch.providers"]
        customOrder["规范化 provider 顺序\nprofile auto 可显式覆盖"]
    end

    policy["FetchPolicy\nprofile + URL intent"]:::policy
    clients["tools.WebFetchClients\nGitHub, Firecrawl, Jina, TinyFish, Exa, Tavily"]:::core
    fetchProviders["fetchProviders(clients)"]:::core
    router["router.Run(WebFetch)"]:::core

    github["GitHub Provider\nrepo metadata + README"]:::provider
    jina["Jina Reader\ncheap profile first"]:::provider
    tiny["TinyFish Fetch\nmulti-key pool"]:::provider
    fire["Firecrawl Scrape\nquality profile first when configured"]:::provider
    exa["Exa Contents"]:::provider
    tavily["Tavily Extract"]:::provider

    output["WebFetchResult\nsource + url + content + route_trace"]:::output

    config --> v1 --> defaultOrder --> policy --> clients
    config --> v2 --> customOrder --> policy --> clients
    clients --> fetchProviders --> router
    router --> github
    router --> jina
    router --> tiny
    router --> fire
    router --> exa
    router --> tavily
    jina --> output
    tiny --> output
    fire --> output
    exa --> output
    tavily --> output

    classDef config fill:#f3f4f6,stroke:#6b7280,color:#111827;
    classDef policy fill:#ede9fe,stroke:#7c3aed,color:#2e1065;
    classDef core fill:#dcfce7,stroke:#16a34a,color:#052e16;
    classDef provider fill:#fff7ed,stroke:#ea580c,color:#431407;
    classDef output fill:#ecfccb,stroke:#65a30d,color:#1a2e05;
```

Firecrawl / profile fetch 规则：

- `fetch --profile auto`：GitHub repo/blob/tree/issues/releases URL 先走 GitHub Provider；普通网页默认 Firecrawl -> Jina -> Exa -> Tavily -> TinyFish。
- `fetch --profile quality`：普通网页走质量优先顺序。
- `fetch --profile cheap`：Jina -> Firecrawl -> Exa -> Tavily。
- V2：`capabilities.web_fetch.providers` 可显式指定 `auto` 的普通 provider 顺序。
- Firecrawl 只有在启用且带有 key 时才会实际参与；未配置时 provider 会被跳过。
- 共享 fetch 路由里，Firecrawl 使用 `FirecrawlPool`，每次轮换起始 key；遇到上游错误或空 markdown，会继续尝试剩余 key。
- 直接 `firecrawl-scrape` 和 `firecrawl-map` 当前通过 `buildFirecrawlClient` 使用第一个规范化后的 Firecrawl key。

代码位置：

- V1/V2 配置规范化：`internal/config/config.go`
- 共享 fetch workflow：`internal/tools/fetch.go`
- CLI fetch 命令：`internal/cli/fetch.go`
- Fetch adapters：`internal/router/adapters/fetch.go`
- Firecrawl pool 和 client：`internal/engine/firecrawl.go`

## Research 和 Smart Answer

`research` 与 `smart-answer` 是组合 workflow。它们不定义新的 search/fetch provider 顺序，而是复用共享 search 和 fetch 路径。

```mermaid
flowchart TB
    query["research query"]:::input
    plan["BuildSearchPlan\n提取 web_search 查询"]:::step
    searches["并发 web_search 调用\nprofile 默认 auto"]:::step
    sources["source cache + source URLs"]:::step
    rank["去重、domain filter、\n排序高信号 URL"]:::step
    fetch["并发 web_fetch 调用"]:::step
    site["可选 web_map\n和 deep web_crawl"]:::step
    pack["ResearchPack\nfacts, inferences,\nopen questions"]:::output

    smart["smart_answer"]:::smart
    reason["ReasoningPool\nreasoningEndpoints[]"]:::provider
    answer["最终答案"]:::output

    query --> plan --> searches --> sources --> rank --> fetch --> pack
    rank --> site --> pack
    pack --> smart --> reason --> answer

    classDef input fill:#f3f4f6,stroke:#6b7280,color:#111827;
    classDef step fill:#e0f2fe,stroke:#0284c7,color:#082f49;
    classDef smart fill:#ede9fe,stroke:#7c3aed,color:#2e1065;
    classDef provider fill:#fff7ed,stroke:#ea580c,color:#431407;
    classDef output fill:#ecfccb,stroke:#65a30d,color:#1a2e05;
```

代码位置：

- Research executor 组装：`internal/tools/research.go` 里的 `NewResearchExecutor`
- CLI research 入口：`internal/cli/research.go`
- MCP research tool 注册：`internal/tools/research.go` 里的 `RegisterResearchRun`
- Smart answer 组合：`internal/tools/smart_answer.go`
- 推理 endpoint pool：`internal/engine/reasoning.go`

## Firecrawl 变更视图

Firecrawl 的接入范围很窄：直接 CLI 命令用于难抓页面和站点结构；顶层 `firecrawl` 配置启用后可以参与共享 fetch，v2 配置用于显式改写 provider 顺序。

```mermaid
flowchart LR
    cfg["当前配置"]:::config

    subgraph DirectCLI["直接 CLI 命令"]
        scrape["firecrawl-scrape <url>"]:::direct
        fmap["firecrawl-map <url>"]:::direct
        client["FirecrawlClient\n第一个规范化 key"]:::engine
    end

    subgraph OptionalFetch["可选共享 fetch provider"]
        v2["firecrawl config\ntop-level or v2 provider"]:::config
        pool["FirecrawlPool\n轮换起始 key"]:::engine
        adapter["FirecrawlFetchProvider\nonlyCleanContent=true"]:::direct
        fetchRoute["共享 web_fetch 路由"]:::core
    end

    fc["Firecrawl API v2"]:::provider

    cfg --> scrape --> client --> fc
    cfg --> fmap --> client
    cfg --> v2 --> pool --> adapter --> fetchRoute --> fc

    classDef config fill:#f3f4f6,stroke:#6b7280,color:#111827;
    classDef direct fill:#e0f2fe,stroke:#0284c7,color:#082f49;
    classDef engine fill:#dcfce7,stroke:#16a34a,color:#052e16;
    classDef core fill:#ede9fe,stroke:#7c3aed,color:#2e1065;
    classDef provider fill:#fff7ed,stroke:#ea580c,color:#431407;
```

当前边界：

- 已有专用 Firecrawl CLI 命令：`internal/cli/firecrawl.go`。
- `internal/server/server.go` 没有注册专用 Firecrawl MCP tool。
- 顶层 `firecrawl` 配置启用并带有 key 后，Firecrawl 可以参与 `web_fetch`；v2 配置可显式排序。
- 现有 `map` 和 `crawl` 仍由 Tavily 支撑。
- 现有 `search` 不包含 Firecrawl。
- Firecrawl 单元测试使用本地 test server，不能调用真实 Firecrawl API。

## Provider 矩阵

| Provider | 搜索 | 抓取 | 文档 | Map/Crawl | 推理 | 配置/key 说明 |
| --- | :---: | :---: | :---: | :---: | :---: | --- |
| Grok / OpenAI-compatible pool | 是 | 否 | 否 | 否 | 否 | `grokEndpoints[]`；search profiles 在这里配置 |
| TinyFish | fallback | fallback | 否 | 否 | 否 | `tinyfish.keys[]`；multi-key pool |
| Exa | fallback | fallback | 是 | 否 | 否 | `exa.apiKey`；也有 advanced direct tools |
| Tavily | fallback | fallback | 否 | 是 | 否 | `tavily.apiKey`；direct map/crawl |
| Jina Reader | 否 | cheap profile 第一位 / quality fallback | 否 | 否 | 否 | 可不带 key 使用 |
| Firecrawl | 否 | quality profile 第一位（配置 key 后） | 否 | 仅直接 CLI map | 否 | 直接 CLI 命令要求 `firecrawl.enabled=true`；共享 fetch 支持 policy-first 和显式 v2 provider 顺序 |
| Reasoning endpoints | 否 | 否 | 否 | 否 | 是 | `reasoningEndpoints[]`；只给 `smart_answer` 使用 |

## 更新规则

下面这些位置改动时，顺手更新这份文档：

- `internal/app/app.go`：顶层 command/server 路由。
- `internal/server/server.go`：MCP tool 注册或共享 client wiring。
- `internal/cli/cli.go` 或 `internal/cli/providers.go`：CLI command surface 或生产 client 构造。
- `internal/tools/*`：共享 search/fetch/research/smart-answer 行为。
- `internal/router/*` 或 `internal/router/adapters/*`：fallback 语义或 provider 顺序。
- `internal/config/config.go`：配置 schema、v1/v2 规范化、默认 provider 顺序、profile policy。
- `internal/engine/*`：provider client 行为、key pool、retry、输出 helper。
- `README.md`、`docs/AI_USAGE.md` 和示例配置：公开行为变化时同步。

判断标准很简单：只要 feature 改了 command 路由、MCP tool 暴露、provider fallback 顺序、配置 schema 或公开行为，就在同一个变更里更新相关图和 provider 矩阵行。
