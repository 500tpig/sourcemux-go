# research_run 并发化与 per-call 超时

## Goal

deep 模式 `research_run` 端到端动辄 3–5 分钟、串行调用拖死 CLI 默认 180s 超时；
本任务改为并发执行 plan 内 search 与 fetch，并对每次子调用加固定 per-call 超时，让
最差耗时 ≈ 单次最慢调用而非求和。

## Requirements

- `ResearchExecutor.Run` 内 `PlanQueries` 的 search 改为并发，并发度按 depth 区分
  （quick=2, standard=3, deep=4），保持 `pack.ExecutedSearches` 的输出顺序与
  `PlanQueries` 一致。
- `fetchSelectedSources` 改为并发，并发度同上；输出顺序与 `selectedFetchURLs` 一致。
- 每次 Search/Fetch 调用包一层 `context.WithTimeout(ctx, 25s)`，避免单慢端点吃掉
  整体预算。
- `mapTargetDomains` / `crawlTargetDomains` 暂不并发化（每次最多 1–2 调用，
  改造收益小，留给后续）。
- 所有现有 `internal/tools/research_test.go` 测试通过；新增针对并发顺序与
  per-call 超时行为的 1–2 个用例。

## Acceptance Criteria

- [ ] `go test ./internal/tools/... -run Research -count=1` 全绿。
- [ ] `go test ./...` 全绿。
- [ ] `go build ./...` 通过。
- [ ] 新增测试覆盖：(a) 并发执行下顺序保持；(b) per-call 超时触发不影响其他子调用。

## Definition of Done

- 不引入新依赖（仅 stdlib `sync`）。
- per-call 超时为常量，便于后续调参。
- 不修改 `ResearchOptions` / `ResearchPack` 的 JSON 字段名或顺序。

## Technical Approach

- 新增 `researchPerCallTimeout = 25 * time.Second`（常量）。
- 新增 `researchConcurrency(depth)` -> int。
- search 阶段：先按 plan 顺序构造 `searchSummaries := make([]ResearchSearchSummary, len(PlanQueries))` 与
  `perQueryURLs := make([][]researchSourceInput, len(PlanQueries))`；用
  `sync.WaitGroup` + 缓冲 channel（信号量）并发跑；写入下标固定，主循环回收时
  按索引顺序拼回 `pack.ExecutedSearches` 与 `sourceInputs`。
- fetch 阶段同样按下标并发，结果写入预分配的 `[]ResearchFetchedPage`。
- 每个子调用入口：`subCtx, cancel := context.WithTimeout(ctx, researchPerCallTimeout); defer cancel()`。

## Out of Scope

- 调整 BuildSearchPlan 输出。
- map/crawl 并发化。
- CLI 默认 timeout 调整（暂沿用 180s，并发化后已足够）。

## Technical Notes

- 关键文件：`internal/tools/research.go:248`（Run）、`:286`（search loop）、
  `:373`（fetchSelectedSources）。
- 测试参考：`internal/tools/research_test.go:87` (`TestResearchExecutorBuildsStablePack`)
  使用 `context.Background()`，per-call 25s 不会误触发。
- 没有 `golang.org/x/sync/errgroup` 依赖；用 stdlib `sync.WaitGroup` + 信号量 channel 实现。
