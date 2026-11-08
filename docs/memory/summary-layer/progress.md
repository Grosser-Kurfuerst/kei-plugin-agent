# memory/summary-layer/progress.md — 摘要层实现进度

本文跟踪摘要层 S1–S7（[`summary-layer.md`](summary-layer.md) §16.3）的实现进度。

**更新规则**

- 阶段状态只用三个词：`未开始`、`进行中`、`已完成`。
- `已完成` 必须满足 [`summary-layer.md`](summary-layer.md) §16.3.2 的完成条件：本阶段验收表全过、运行验收走通、质量门全绿、§16.3.10 的文档已同步。
- 阶段完成后勾选下方清单、填写完成日期与提交，并同步 [`../progress.md`](../progress.md) 的 M1 / M2 行：M1 = S1–S2，M2 = S3–S7。
- 实现与设计不一致时，先改设计文档，再在「偏差记录」中记一条。

## 阶段进度

| 阶段 | 所属 | 状态 | 开始日期 | 完成日期 | 提交 / PR | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| S1 存储与写入 | M1 | 已完成 | 2026-11-05 | 2026-11-05 | `feat/memory` e965e96 … 171d7bf | 未接入根包；golangci-lint 0 issues |
| S2 根包接入采集 | M1 | 已完成 | 2026-11-05 | 2026-11-05 | `feat/memory` 8568c53 … d14dddf | 运行验收用最小 kei 宿主 + mock 适配器 + 本地 PostgreSQL 走通；golangci-lint 0 issues；2026-11-08 在 SnowLuma + PostgreSQL 测试部署（[`../deploy.md`](../deploy.md)）上复验通过：部署验收、运行中断库、启动时库不可达与恢复、DB 单测 |
| S3 任务框架与清理 | M2 | 已完成 | 2026-11-08 | 2026-11-08 | `feat(memory): add scheduled jobs and retention cleanup` | 真实宿主分钟调度与 status 验收通过；质量门全绿，review 2 轮通过 |
| S4 日摘要流水线 | M2 | 已完成 | 2026-11-08 | 2026-11-08 | `4058e6d` + `fix(memory): bound summary message and overlap budgets` | 预算规则已确认；真实会话日与真实模型抽验通过；本轮质量门全绿、review 1 轮通过 |
| S5 读取与注入 | M2 | 未开始 | — | — | — | — |
| S6 滚动摘要与显式查询 | M2 | 未开始 | — | — | — | — |
| S7 评测与调参 | M2 | 未开始 | — | — | — | — |

## 阶段清单

### S1 存储与写入（§16.3.3）

- [x] `go.mod` 引入 `gorm.io/gorm`、`gorm.io/driver/postgres`
- [x] 类型、`conv_key`、模型与迁移、写协程、连库状态机与 `Stop`
- [x] `memtest.NewDB` 与测试库启动说明
- [x] 验收表全部通过
- [x] 质量门全绿
- [x] 文档同步：`architecture.md` 第 5 章、`testing.md` §11.1

### S2 根包接入采集（§16.3.4）

- [x] 记录构造、入队点、生命周期接入
- [x] 配置键与校验、`/agent memory status` 两行
- [x] 验收表全部通过
- [x] 运行验收：落库、重复投递、数据库宕机与恢复、启动时数据库不可达
- [x] 质量门全绿
- [x] 文档同步：`configuration.md`、`architecture.md` 第 1 章 / §3.1 / §3.2 / 第 5 章、`persona.md` §8.4、`testing.md` §11.2

### S3 任务框架与清理（§16.3.5）

- [x] `mem_job`、领取与重试、启动时重置执行中任务、worker 池、调度 tick、`cleanup`
- [x] 配置键、status「任务」行
- [x] 验收表全部通过
- [x] 运行验收：到点后 `cleanup` 任务完成
- [x] 质量门全绿
- [x] 文档同步：`configuration.md`、`persona.md` §8.4、`architecture.md` §3.2

### S4 日摘要流水线（§16.3.6）

- [x] 摘要与话题表、LLM 客户端、按会话建日摘要任务并逐天处理
- [x] 切片、预过滤、切片小结、合并与兜底、写入
- [x] 提示词全文完成并评审
- [x] 验收表全部通过
- [x] 运行验收：真实数据生成昨天的日摘要，人工抽读无明显编造或归属错误
- [x] 质量门全绿
- [x] 文档同步：`configuration.md` 的 `memory_llm_*`、`testing.md` §11.2

验收证据：

- `TestRuntimeDailySummary`：经用户明确批准，将一个已采集会话日回放至隔离 schema，调用现有配置的 `api.deepseek.com`；25 条消息生成 3 个话题，25.83s 通过，prompt / completion tokens = 1476 / 4168。引用归属校验通过；对照来源抽读，主体与核心话题来自原文。单日运行抽验不代表 S7 正式质量指标达标。
- `TestScheduledDailySummary`：真实 Service 调度、worker、HTTP 桩、摘要与 `mem_job.result` 集成验证通过；与上述真实模型 Handler 回放共同提供运行证据，不将二者混称一次完整宿主验收。
- 用户确认预算优先；单条正文 2000 rune（含省略号），切片正文 8000 rune，重叠最多 10 条且按需缩减，文档与边界测试已同步。

### S5 读取与注入（§16.3.7）

- [ ] `Context`、关键词相关度与打分、选取与行渲染
- [ ] 根包取上下文、记忆块渲染、`renderSystemPrompt` 迁移
- [ ] 配置键
- [ ] 验收表全部通过
- [ ] 运行验收：能接上前几天的话题；数据库宕机时照常回复
- [ ] 质量门全绿
- [ ] 文档同步：`configuration.md`、`persona.md` §8.5、`architecture.md` 第 2 章

### S6 滚动摘要与显式查询（§16.3.8）

- [ ] 滚动摘要触发与流水线、读取侧滚动候选与短期窗口去重
- [ ] `QuerySummaries`、`memory__summary`、`toolsFor` 迁移
- [ ] 配置键与人格 `tools` 校验
- [ ] 验收表全部通过
- [ ] 运行验收：当天出现滚动摘要、次日被替换；模型调用 `memory__summary`
- [ ] 质量门全绿
- [ ] 文档同步：`configuration.md`、`architecture.md` 第 1 / 5 章，核对 `mcp.md`

### S7 评测与调参（§16.3.9）

- [ ] 20 个脱敏评测 case 与标注
- [ ] `TestEvalDaily` 与报告输出
- [ ] `testdata/eval/RESULTS.md` 最新记录达标：覆盖率 ≥ 80%、归属正确率 ≥ 95%、无编造
- [ ] 调参后 S4–S6 测试仍通过

## 评测结果摘要

详细记录在 `internal/memory/summary/testdata/eval/RESULTS.md`（S7 创建），这里只登记最新一次的结论。

| 日期 | 覆盖率 | 归属正确率 | 编造 | 平均 token / 会话日 | 备注 |
| --- | --- | --- | --- | --- | --- |
| — | — | — | — | — | 尚未评测 |

## 偏差记录

| 日期 | 阶段 | 偏差 | 处理 |
| --- | --- | --- | --- |
| 2026-11-05 | S1 | 实现时补全的细节：`conv_key` 也要求 `Platform` 非空；群会话行不记 `bot_id`；`At` 为零值时用写入时刻；DSN 无法解析时记 `Error` 并退出、状态保持「连接中」；新增常量 `migrateTimeout = 30s`；连库、迁移、写入由同一个后台协程依次执行；被 `Stop` 打断的批次交给 flush 重写；gorm 日志适配器命名为 `store.NewLogger` | 已同步到 [`summary-layer.md`](summary-layer.md) §16.3.3 |
| 2026-11-05 | S1 | 按「不留占位」，`New` 在 S1 不带 `httpClient`（S4 引入 LLM 客户端时再加）；`prepare` 清洗 `Text` 与名字中的非法 UTF-8 和 `\x00`，`Enqueue` 复制 `Mentions`；数据库从未就绪时 `Stop` 对队列残留记 `Warn` | 已同步到 [`summary-layer.md`](summary-layer.md) §16.2.4、§16.3.3、§16.3.4、§16.3.6 |
| 2026-11-05 | S2 | 实现时补全的细节：`p.mem.Start` 放在 MCP 目录加载成功之后；`memEpoch` 在 `Start` 记录；`/agent memory` 不建会话状态，缺参或参数非法回用法文本；自身回复的 `Conv` 取自发送目标（与 `channelState` 同源） | 已同步到 [`summary-layer.md`](summary-layer.md) §16.3.4 |
| 2026-11-08 | S1 | 远程 PostgreSQL 上 `TestConcurrentEnqueueAndStop` 稳定失败（`Written` 少一批、`Duplicates` 多一批）：`Stop` 取消 ctx 时正在 COMMIT 的批次服务端已提交、客户端报错，flush 重写后整批记成重复。改为 `Stop` 后正在写的批次与 flush 共用 `flushTimeout` 收尾时限，不再立即打断 | 已同步到 [`summary-layer.md`](summary-layer.md) §16.3.3 |

## 迭代验证记录

- S3（2026-11-08）：Nix devShell / `GOTOOLCHAIN=local` 下 `gofmt`、build、vet、带 PostgreSQL 独立 schema 的 test / race、lint 全通过（0 issues）。`TestRuntimeHostCleanup` 用真实 devhost、mock 控制面与分钟时钟，60.10s 通过：cleanup 完成且 status 显示任务行。review 2 轮，最终 `NO_BLOCKING_ISSUES`；修复状态落库失败重试、未启用记忆时的新增配置校验，补齐清理与停止验证；无未复评修复。

- S4（2026-11-08，代码提交）：Nix devShell 下 build、vet、带 PostgreSQL 独立 schema 的 test / race、lint 全通过，0 issues，gofmt / diff check 干净。review 2 轮，代码最终 `NO_BLOCKING_ISSUES`；修复 JSON 修复响应的旧字段残留和任务超时时已完成调用的用量漏记，均有回归测试且经复评。未完成真实数据验收，阶段保持进行中；S5 尚未开始。

- S4 收尾（2026-11-08）：按用户补充要求实现切片预算与单条截断规则，build / vet / 全仓 PostgreSQL race / lint 全过，0 issues。真实数据验收与来源抽读完成；本轮 review 1 轮，最终 `NO_BLOCKING_ISSUES`，无未复评修复。
