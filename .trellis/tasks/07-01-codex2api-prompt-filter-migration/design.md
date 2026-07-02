# 迁移 codex2api Prompt 本地审计策略到风控中心 - Technical Design

## Boundary

本设计只在现有风控中心中新增一层“本地 Prompt 规则策略”，不替换当前 OpenAI Moderations 审核链路，不新增第二套 Moderations API 配置，不改变用户请求转发、账号调度、计费、错误透传和上游 `cyber_policy` 处理。

需要保持低冲突：
- 后端主流程 `content_moderation.go` 只新增小接入点和必要 side-effect 解耦。
- codex2api 规则引擎、规则数据、完整上下文抽取放到新文件。
- 前端在 `RiskControlView.vue` 中新增独立配置区，不重写现有风控页面。
- 配置继续存入现有 `content_moderation_config` JSON。
- 日志继续写入现有 `content_moderation_logs`，只追加兼容字段。

明确不做：
- 不迁移 codex2api 的独立 review API 配置。
- 不把 `keyword_blocking_mode` 改造成总审计策略。
- 不修改 codex2api 内置规则的安全边界和正则含义。
- 不新增并行自动封禁系统。

## Design Decisions Locked

以下决策已经收敛，后续实现不得再引入新的组合模式或隐式行为：

- 本地 Prompt 策略默认关闭，升级后不改变任何请求行为。
- 管理员开启本地策略后，codex2api 内置规则默认全部启用。
- 本地策略首次开启默认只记录：`action=record`。
- 扫描范围默认 `latest_user_input`；`full_text_context` 作为可选增强模式。
- 本地规则命中但未拦截时，默认继续调用当前风控中心已有的 OpenAI Moderations API。
- 自动封禁计数、风险哈希、邮件通知都是独立副作用开关，默认全部关闭。
- `keyword_blocking_mode` 继续只表达“关键词列表 + OpenAI Moderations API”的关系，不控制 codex2api 本地规则。
- 本地规则 action 固定为 `local_rule_hit` / `local_rule_block`，不复用 `matched_keyword` 或 `block`。
- `sample_rate` 只影响 OpenAI Moderations API 抽样，不影响本地规则扫描。
- 当前没有需要用户继续决策的产品问题；剩余风险都应在实现和测试阶段处理。

## Codex2api Reference Mapping

迁移参考以 codex2api 当前公开实现为准：
- `security/promptfilter/filter.go`
  - `DefaultThreshold=50`
  - `DefaultStrictThreshold=90`
  - `DefaultMaxTextLength=80*1024`
  - `defaultHeadScanLength=64*1024`
  - `defaultTailScanLength=16*1024`
  - `ExtractText` 按 endpoint 抽取 `messages/system/prompt/input/instructions/style` 等文本。
  - `collectGJSONText` 递归抽取 `text/content`，并跳过 `image_url/url/file_id/result/data/b64_json/source/file/type/role`。
  - `limitScanText` 对超长文本保留头尾，且通过 `safeUTF8Prefix/safeUTF8Suffix` 保证 UTF-8 安全。
  - `normalizeForScan` 去代码围栏、控制字符、大小写和多余空白。
  - `defensiveContextDiscount` 对防御、检测、修复类上下文降分。
- `security/promptfilter/patterns.go`
  - `defaultPatternConfigs` 是本次迁移的内置规则来源。
  - `sensitiveRedactionPatterns` 是脱敏规则来源。
  - `defensiveContextPatterns` 是防御语境降分来源。

本项目映射关系：
- codex2api `ModeMonitor/ModeWarn/ModeBlock` 不直接迁移为三态模式；本项目只需要 `record/block` 两个本地动作，且受现有 `observe/pre_block/off` 总模式约束。
- codex2api `ActionAllow/ActionWarn/ActionBlock` 不直接暴露给管理员；本项目日志动作统一为 `local_rule_hit/local_rule_block`。
- codex2api `ReviewConfig` 不迁移；本项目继续复用现有 OpenAI Moderations API 配置，即当前风控中心的 `base_url/model/api_keys/timeout/retry/worker`。
- codex2api `SensitiveWords` 暂不作为首期 UI 独立字段迁移；管理员需要扩展时使用本项目的自定义规则或现有关键词能力。
- codex2api 的完整文本抽取能力映射到本项目 `scan_scope=full_text_context`；默认仍使用本项目现有 `latest_user_input`，降低升级后的行为变化。

codex2api 扫描更完整文本上下文的原因：
- 网关层最容易拿到完整请求体，可以在进入上游模型前统一做本地安全判断。
- 风险 Prompt 可能出现在 `system/instructions`、历史 `messages`、嵌套 `content/text` 或 Responses `input` 中，只扫描最新用户输入会漏掉上下文注入和多轮铺垫。
- 它没有扫描原始 JSON 全文，而是抽取结构化可读文本并跳过高噪声字段；本项目的 `full_text_context` 必须保留这一点。

## Independent Control Dimensions

本地 Prompt 策略必须拆成独立维度，避免未来出现含义不清的组合枚举：

| 维度 | 配置 | 默认值 | 作用边界 |
|---|---|---|---|
| 总开关 | `local_rules.enabled` | `false` | 控制本地规则是否运行；不影响关键词和 OpenAI Moderations API。 |
| 规则集 | 内置规则 + `disabled_builtin_rules` + `custom_rules` | 内置全启用，自定义空 | 控制哪些正则参与评分；不表达命中后动作。 |
| 扫描范围 | `scan_scope` | `latest_user_input` | 控制扫描文本来源；不改变日志字段和副作用。 |
| 评分阈值 | `threshold/strict_threshold` | `50/90` | 控制是否命中；不控制是否拦截。 |
| 处理动作 | `action` | `record` | 控制命中后在允许同步拦截的路径中只记录还是阻断。 |
| API 联动 | `skip_api_after_hit` | `false` | 仅控制本地规则命中但未阻断时是否跳过现有 OpenAI Moderations API。 |
| 自动封禁计数 | `count_for_auto_ban` | `false` | 控制本地规则日志是否进入现有封禁累计查询。 |
| 风险哈希 | `record_hash` | `false` | 控制本地规则命中是否写入现有 Redis 风险哈希。 |
| 邮件通知 | `email_on_hit` | `false` | 控制本地规则命中是否复用现有内容审计邮件通知。 |
| 日志详情 | `local_rule_detail` | 命中时写入 | 记录规则、分数、分类、脱敏预览；不保存完整请求体。 |

组合约束：
- `action=block` 只有在现有风控总模式为 `pre_block` 时才会同步阻断；`observe` 下只能记录。
- `skip_api_after_hit=false` 表示默认继续现有 OpenAI Moderations API，不新增第二套审核 API。
- `keyword_only` 下 OpenAI Moderations API 本来不在路径中，本地规则不得因为 `skip_api_after_hit=false` 重新打开 API。
- `count_for_auto_ban/record_hash/email_on_hit` 互不绑定，允许管理员分别灰度。

## Current Code Facts

当前风控执行流集中在 `backend/internal/service/content_moderation.go`：
1. 读取 `risk_control_enabled`。
2. 读取 `ContentModerationConfig`。
3. 判断 `enabled`、`mode`、分组范围、模型范围。
4. `ExtractContentModerationInput` 提取当前最新用户输入。
5. `pre_block` 下先执行关键词拦截。
6. `keyword_only` 在 `pre_block` 中关键词未命中后直接放行。
7. 风险哈希检查。
8. `sample_rate` 抽样。
9. Moderations API Key 检查。
10. `observe` 异步调用 OpenAI Moderations，`pre_block` 同步调用 OpenAI Moderations。

当前关键词模式事实：
- `keyword_and_api`、`keyword_only`、`api_only` 是关键词列表与 OpenAI Moderations API 的联动策略。
- 当前代码中关键词策略只在 `pre_block` 下同步生效；前端已有“关键词拦截仅在前置拦截模式下生效”的提示。
- 新本地规则必须在 `keyword_only` 的 return 之前运行，否则 `keyword_only` 下本地规则永远没有机会执行。

当前副作用事实：
- `persistContentModerationLog(ctx, cfg, log, hashText, recordHash, applySideEffects)` 用 `applySideEffects` 同时控制自动封禁计数和邮件发送。
- 本地策略要求“计入自动封禁”“写风险哈希”“发送邮件”拆成独立维度，因此需要引入更细的 side-effect policy。

## Architecture

新增后端模块：
- `backend/internal/service/content_moderation_local_rules.go`
  - 本地规则配置、归一化、校验、评分结果、测试接口、主流程接入 helper。
- `backend/internal/service/content_moderation_local_rule_patterns.go`
  - codex2api 内置规则数据，保留 `name/pattern/weight/category/strict`。
- `backend/internal/service/content_moderation_local_rule_extract.go`
  - `latest_user_input` 与 `full_text_context` 扫描文本抽取、跳过字段、头尾截断、UTF-8 安全截断。
- `backend/internal/service/content_moderation_local_rule_engine.go`
  - 正则预编译、配置缓存、匹配评分、防御语境降分、脱敏命中片段。
- 可选新增 `backend/internal/service/content_moderation_side_effects.go`
  - 把日志持久化副作用从一个 bool 拆成明确 policy，减少 `content_moderation.go` 膨胀。

保持现有文件职责：
- `content_moderation.go` 仍是风控主编排，新增 `checkLocalRules(...)` 调用点。
- `content_moderation_input.go` 保持现有最新用户输入提取能力；`full_text_context` 不强行塞入此文件，避免混淆两种抽取语义。
- `content_moderation_repo.go` 扩展日志字段读写和封禁计数过滤。
- `admin/content_moderation_handler.go` 透传新增配置和新增测试接口。

## Configuration Contract

在 `ContentModerationConfig` 内新增嵌套字段：

```go
type ContentModerationLocalRulesConfig struct {
    Enabled              bool                                 `json:"enabled"`
    Action               string                               `json:"action"` // record | block
    ScanScope            string                               `json:"scan_scope"` // latest_user_input | full_text_context
    Threshold            int                                  `json:"threshold"`
    StrictThreshold      int                                  `json:"strict_threshold"`
    MaxTextLength        int                                  `json:"max_text_length"`
    SkipAPIAfterHit      bool                                 `json:"skip_api_after_hit"`
    CountForAutoBan      bool                                 `json:"count_for_auto_ban"`
    RecordHash           bool                                 `json:"record_hash"`
    EmailOnHit           bool                                 `json:"email_on_hit"`
    DisabledBuiltinRules []string                             `json:"disabled_builtin_rules"`
    CustomRules          []ContentModerationLocalRulePattern  `json:"custom_rules"`
}

type ContentModerationLocalRulePattern struct {
    Name     string `json:"name"`
    Pattern  string `json:"pattern"`
    Weight   int    `json:"weight"`
    Category string `json:"category,omitempty"`
    Strict   bool   `json:"strict,omitempty"`
    Enabled  *bool  `json:"enabled,omitempty"`
}
```

常量：
- `ContentModerationLocalRulesActionRecord = "record"`
- `ContentModerationLocalRulesActionBlock = "block"`
- `ContentModerationLocalRulesScanLatestUserInput = "latest_user_input"`
- `ContentModerationLocalRulesScanFullTextContext = "full_text_context"`

默认值：
- `enabled=false`
- `action=record`
- `scan_scope=latest_user_input`
- `threshold=50`
- `strict_threshold=90`
- `max_text_length=80*1024`
- `skip_api_after_hit=false`
- `count_for_auto_ban=false`
- `record_hash=false`
- `email_on_hit=false`
- `disabled_builtin_rules=[]`
- `custom_rules=[]`

设计理由：
- `skip_api_after_hit` 使用 false 零值表示继续调用现有 OpenAI Moderations API，旧配置缺字段时行为安全。
- 内置规则用 `disabled_builtin_rules` 表达禁用项，空列表代表全部启用；未来 codex2api 增加规则时不需要给每条规则写配置。
- 自定义规则沿用 codex2api 的 `Enabled *bool` 语义；`nil` 视为启用，显式 false 才禁用。

配置更新语义：
- `UpdateContentModerationConfigInput.LocalRules` 必须是指针字段；请求没有传 `local_rules` 时保持已有本地策略配置不变，避免旧前端保存配置时清空新字段。
- 请求传入 `local_rules` 时，按本地策略子树整体更新；子字段缺失时使用本地策略默认值归一化。
- 前端保存本地策略 tab 时必须提交完整 `local_rules` 子对象，不能只提交局部字段补丁。
- `ContentModerationConfigView` 返回归一化后的完整 `local_rules`，供前端作为编辑基线。

内置规则身份：
- 内置规则以 `name` 作为稳定禁用键。
- `disabled_builtin_rules` 中未知规则名在运行时忽略但保留，避免未来规则集变动或回滚时丢配置。
- UI 只允许切换当前后端返回的 `builtin_rules`；不提供手写未知规则名入口。
- 自定义规则名不得与当前内置规则重名；否则日志、禁用和测试结果会产生歧义。

配置归一化：
- `normalize()` 中调用 `normalizeLocalRulesConfig`。
- `StrictThreshold < Threshold` 时提升到 `Threshold`。
- `Threshold <= 0` 使用默认 50；上限建议 500。
- `StrictThreshold <= 0` 使用默认 90；上限建议 1000。
- `MaxTextLength <= 0` 使用 80 KiB；上限建议 1 MiB。
- `DisabledBuiltinRules` 按大小写不敏感去重、排序。
- 自定义规则保存前 trim，跳过空 name/pattern/weight<=0 的规则，或者在 validate 阶段拒绝不完整规则。

配置校验：
- `action` 只能是 `record` 或 `block`。
- `scan_scope` 只能是 `latest_user_input` 或 `full_text_context`。
- 自定义 regex 必须能 `regexp.Compile`。
- 自定义规则名大小写不敏感去重；不得与内置规则重名，避免禁用和日志展示歧义。
- custom rule weight 必须 > 0，建议上限 500。

`ContentModerationConfigView`：
- 返回归一化后的 `local_rules`。
- 额外在 `local_rules.builtin_rules` 中返回只读内置规则列表，供前端展示和禁用。
- `UpdateContentModerationConfigInput` 和 handler request 只接收可写配置，忽略只读 `builtin_rules`。

## Local Rule Engine

规则引擎接近 codex2api：
- 合并内置规则和自定义规则。
- 按 `disabled_builtin_rules`、`enabled=false` 过滤。
- 预编译正则。
- 根据配置构造 cache key，用 `sync.Map` 缓存引擎，避免请求路径重复编译。
- 使用 codex2api 的必含字面量预过滤思路：从 regex AST 提取必含 literal，匹配不到时跳过 regex，降低热路径成本。
- 扫描前执行 `normalizeForScan`：去掉代码围栏、控制字符转空格、转小写、压缩空白。
- 命中规则按 name 去重，分数累加。
- `raw_score` 是降分前总分。
- 防御语境命中时扣分，默认沿用 codex2api 的 30 分规则，分数不低于 0。
- `strict_score` 是 strict 规则权重累加。
- `strict_hit = strict_score >= strict_threshold`。
- 最终命中条件：`score >= threshold || strict_hit`。

结果结构：

```go
type ContentModerationLocalRuleResult struct {
    Enabled        bool
    Hit            bool
    Action         string
    Score          int
    RawScore       int
    StrictScore    int
    Threshold      int
    StrictThreshold int
    StrictHit      bool
    HighestCategory string
    Matches        []ContentModerationLocalRuleMatch
    TextPreview    string
    ContextPreview string
    ExtractedChars int
    ScanScope      string
}

type ContentModerationLocalRuleMatch struct {
    Name     string `json:"name"`
    Weight   int    `json:"weight"`
    Category string `json:"category,omitempty"`
    Strict   bool   `json:"strict,omitempty"`
}
```

日志展示分数：
- `local_rule_detail.score/raw_score/strict_score` 保存原始整数分。
- `ContentModerationLog.HighestScore` 存兼容性的归一化值，建议为 `min(1, max(score/threshold, strict_score/strict_threshold))`。
- `HighestCategory` 存最高权重命中规则的 category；无 category 时用 `local_rule`。
- `CategoryScores` 可存 `{"local_rule": normalizedScore}`，详细分类分数进入 `local_rule_detail`，避免与 OpenAI 分类阈值混淆。

## Scan Text Extraction

本地规则支持两种扫描范围。

### latest_user_input

直接复用当前 `ExtractContentModerationInput(input.Protocol, input.Body)` 的 `content.Text`。

特点：
- 默认模式。
- 语义接近当前风控中心。
- 只扫描最新用户输入。
- 不扫描历史消息、system、instructions。
- 不处理图片内容，只扫描文本。

### full_text_context

新增 `ExtractContentModerationFullTextContext(protocol, endpoint string, body []byte, maxLen int) string`。

低冲突接入约束：
- 当前网关统一调用 `checkContentModeration(..., protocol, model, body)`，没有稳定传入原始 URL endpoint。
- 实现优先使用现有 `protocol` 判断抽取分支，不为 full text context 改动所有 handler 签名。
- 如果未来确实需要 endpoint 细分，再在 `ContentModerationInput` 级别扩展可选 endpoint 字段；本任务不做。

抽取规则接近 codex2api：
- Chat Completions：抽取 `messages`。
- Anthropic Messages：抽取 `system` 和 `messages`。
- Images：抽取 `prompt` 和 `style`。
- Responses/default：抽取 `instructions`、`input`、`prompt`、`messages`。
- Gemini 项目扩展：抽取 `contents` / `parts.text`，跳过 `inline_data.data`、`file_data.file_uri` 等非文本字段。

递归规则：
- 数组递归。
- 对象优先读取 `text`。
- 对象读取 `content`，字符串直接加入，数组/对象递归。
- 遍历其他字段递归，但跳过 skip key。
- 字符串 trim 后加入。
- 数字、布尔、null 不加入。

skip key 大小写不敏感：
- `text`
- `content`
- `image_url`
- `url`
- `file_id`
- `result`
- `data`
- `b64_json`
- `source`
- `file`
- `type`
- `role`
- Gemini 扩展：`inline_data`、`inlineData`、`file_data`、`fileData`

长度限制：
- 默认最大 80 KiB。
- 未超长则完整扫描抽取文本。
- 超长时保留头尾，默认接近 64 KiB 头部 + 16 KiB 尾部。
- 如果 max 小于 80 KiB，则约 4/5 头部 + 1/5 尾部。
- 截断必须 UTF-8 安全。

安全要求：
- full text 只用于扫描，不持久化完整文本。
- 日志只保存脱敏 `input_excerpt` 和最多 3 段脱敏命中上下文。
- 必须复用或增强 `redactContentModerationSecrets`，覆盖 Authorization、password/token/api_key/secret、cookie、sk key、JWT、邮箱。

## Execution Flow

主流程推荐改造点：在 `content.Normalize()` 和 `hashText := content.Hash()` 后、现有 `keyword_only` return 前执行本地规则。

伪流程：

```text
load config and scope checks
extract latest content
normalize content
hashText := content.Hash()

if mode == pre_block:
  if keyword mode allows keywords and keyword hit:
    keep current keyword_block behavior and return

localResult := checkLocalRules(...)
if localResult.hit:
  record local rule log with local side-effect policy
  if mode == pre_block and local_rules.action == block:
    return local_rule_block decision
  if shouldSkipModerationAPIAfterLocalHit(cfg):
    return allow

if mode == pre_block and keyword_blocking_mode == keyword_only:
  keep current keyword_only allow return

pre hash check
sample_rate
api key check
observe async API or pre_block sync API
```

`shouldSkipModerationAPIAfterLocalHit`：
- 如果 `local_rules.skip_api_after_hit=true`，且本地规则命中但未同步拦截，则跳过当前 OpenAI Moderations API。
- 如果当前处于 `pre_block` 且 `keyword_blocking_mode=keyword_only`，API 本来就不在路径中；本地规则不得重新打开 API。
- `sample_rate` 继续只控制 OpenAI Moderations API 调用，不控制本地规则扫描。

`observe` 模式：
- 本地规则可以同步运行并记录命中，但不得同步阻断。
- 默认本地命中后仍 enqueue 当前 OpenAI Moderations API。
- 若 `skip_api_after_hit=true`，本地命中后可以跳过异步 API，以节省成本。
- 不命中时保持当前 observe 行为。

`pre_block` 模式：
- 本地规则命中且 `action=block` 时，在 OpenAI Moderations API 前返回阻断。
- 本地规则命中且 `action=record` 时，按配置记录副作用，然后继续或跳过 API。

`off` / `enabled=false` / group out of scope / model out of scope：
- 本地规则不运行。

## Keyword Strategy Interaction

保留现有 `keyword_blocking_mode` 含义：
- `keyword_and_api`：关键词先硬拦截；关键词未命中后可跑本地规则；本地规则未阻断时默认继续 API。
- `keyword_only`：在当前 `pre_block` 语义下，关键词未命中后不调用 API；本地规则必须放在 return 前执行，但不得让 API 重新进入路径。
- `api_only`：关键词列表不生效；本地规则是否运行只由 `local_rules.enabled` 决定。

前端文案需要把当前“审计策略”改成“关键词/API 策略”或“关键词审计策略”，避免管理员误认为它控制本地规则。

## Log Model

新增 action：
- `local_rule_hit`：本地规则命中但未同步阻断。
- `local_rule_block`：本地规则命中并同步阻断。

更新 block 统计：
- `recordPreBlockSyncMetric` 的 blocked 计数加入 `local_rule_block`。
- 日志筛选 `blocked` 加入 `local_rule_block`。

新增日志字段：

```sql
ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS local_rule_detail JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS exclude_from_auto_ban_count BOOLEAN NOT NULL DEFAULT FALSE;
```

设计理由：
- `local_rule_detail` 用 JSONB 承载规则命中详情，后续规则字段扩展低冲突。
- `exclude_from_auto_ban_count` 用零值 false 保持历史和现有 API 命中继续计数；本地策略默认不计入封禁时显式写 true。

`ContentModerationLog` 新增：

```go
LocalRuleDetail           ContentModerationLocalRuleLogDetail `json:"local_rule_detail,omitempty"`
ExcludeFromAutoBanCount   bool                                `json:"exclude_from_auto_ban_count"`
```

`ContentModerationLocalRuleLogDetail`：

```go
type ContentModerationLocalRuleLogDetail struct {
    Source          string                            `json:"source"`
    ScanScope       string                            `json:"scan_scope"`
    Score           int                               `json:"score"`
    RawScore        int                               `json:"raw_score"`
    StrictScore     int                               `json:"strict_score"`
    Threshold       int                               `json:"threshold"`
    StrictThreshold int                               `json:"strict_threshold"`
    StrictHit       bool                              `json:"strict_hit"`
    HighestCategory string                            `json:"highest_category"`
    Matches         []ContentModerationLocalRuleMatch `json:"matches"`
    ContextPreview  string                            `json:"context_preview,omitempty"`
    ExtractedChars  int                               `json:"extracted_chars"`
}
```

仓储变更：
- `CreateLog` marshal `LocalRuleDetail`。
- `ListLogs` scan/unmarshal `local_rule_detail` 和 `exclude_from_auto_ban_count`。
- `CountFlaggedByUserSince` 增加：

```sql
AND exclude_from_auto_ban_count = FALSE
```

保留现有：
- `action <> 'hash_block'`
- `excludeCyberPolicy` 对 `cyber_policy` 的条件排除。

## Side Effects

用 policy 替代单一 `applySideEffects` bool：

```go
type contentModerationSideEffectPolicy struct {
    RecordHash       bool
    CountForAutoBan  bool
    SendEmail        bool
}
```

建议函数：

```go
func defaultModerationSideEffectPolicy(flagged bool) contentModerationSideEffectPolicy {
    return contentModerationSideEffectPolicy{
        RecordHash:      flagged,
        CountForAutoBan: flagged,
        SendEmail:       flagged,
    }
}

func localRuleSideEffectPolicy(cfg ContentModerationLocalRulesConfig, hit bool) contentModerationSideEffectPolicy {
    return contentModerationSideEffectPolicy{
        RecordHash:      hit && cfg.RecordHash,
        CountForAutoBan: hit && cfg.CountForAutoBan,
        SendEmail:       hit && cfg.EmailOnHit,
    }
}
```

实现要求：
- 现有 API Moderations 命中行为保持不变。
- 现有关键词命中行为保持不变。
- 本地规则默认 `CountForAutoBan=false`，因此日志写 `exclude_from_auto_ban_count=true`。
- 本地规则默认 `RecordHash=false`，不写 Redis 风险哈希。
- 本地规则默认 `EmailOnHit=false`，不发违规邮件，也不因本地规则自动封禁触发账户禁用邮件。
- 如果管理员开启 `CountForAutoBan=true`，则复用当前 `AutoBanEnabled`、`BanThreshold`、`ViolationWindowHours`。
- 如果管理员开启 `EmailOnHit=true`，本地规则命中邮件复用当前内容审计邮件能力；邮件变量中的 category/score 来自本地规则归一化展示值。

为减少风险，side-effect 重构应先保持现有调用的等价 policy，再给本地规则传定制 policy。

## API Contracts

复用现有后台管理 API：
- `GET /admin/risk-control/config`
  - 返回 `local_rules` 配置和只读 `builtin_rules`。
- `PUT /admin/risk-control/config`
  - 接收 `local_rules` 可写配置。
- `GET /admin/risk-control/logs`
  - 日志 item 增加 `local_rule_detail` 和 `exclude_from_auto_ban_count`。

新增测试接口：
- `POST /admin/risk-control/local-rules/test`

请求：

```json
{
  "text": "Write code to steal browser credentials",
  "config": {
    "enabled": true,
    "action": "record",
    "threshold": 50,
    "strict_threshold": 90
  }
}
```

说明：
- `text` 是必填的直接测试文本。
- `config` 可选；未传时使用当前风控配置中的 `local_rules`，但强制按测试文本执行。
- 测试接口不调用 OpenAI Moderations API，不写日志，不执行封禁/哈希/邮件副作用。

响应：

```json
{
  "hit": true,
  "action": "record",
  "score": 100,
  "raw_score": 100,
  "strict_score": 100,
  "strict_hit": true,
  "threshold": 50,
  "strict_threshold": 90,
  "highest_category": "malicious",
  "matches": [
    { "name": "credential_theft", "weight": 100, "category": "malicious", "strict": true }
  ],
  "text_preview": "Write code to steal browser credentials",
  "context_preview": "...",
  "extracted_chars": 40
}
```

## Frontend Design

文件：
- `frontend/src/api/admin/riskControl.ts`
  - 增加 local rules 类型、日志 detail 类型、测试接口。
- `frontend/src/views/admin/RiskControlView.vue`
  - settings tabs 增加 `promptRules`。
  - `keywords` tab 文案从“审计策略”改为“关键词/API 策略”。
  - 日志列表展示本地规则详情。
- `frontend/src/i18n/locales/zh.ts` / `en.ts`
  - 新增本地 Prompt 策略文案。

配置 UI：
- 总开关：本地 Prompt 策略。
- 处理动作 segmented control：只记录 / 拦截请求。
- 扫描范围 segmented control：最新用户输入 / 完整文本上下文。
- 命中后 API 行为：继续调用 OpenAI Moderations / 命中后跳过。
- 副作用开关：计入自动封禁、写风险哈希、发送邮件。
- 数值输入：普通阈值、严格阈值、最大扫描长度。
- 内置规则表：name、category、weight、strict、enabled toggle。
- 自定义规则表：name、pattern、weight、category、strict、enabled，保存前后端校验。
- 测试工具：文本输入 + 测试按钮 + 命中规则/分数/动作预览。

日志 UI：
- `local_rule_hit` 显示为“本地规则命中”。
- `local_rule_block` 显示为“本地规则拦截”。
- 展示 score/raw score/strict hit/highest category。
- 展示最多前几条 match name，不直接塞进 `matched_keyword`。
- `matched_keyword` 继续只用于关键词。

交互提示：
- `keyword_only` 下提示：当前关键词策略在前置拦截中不调用 OpenAI Moderations API；本地规则不会重新打开 API。
- `api_only` 下如果本地规则开启，提示：仅 API 只表示关键词列表不生效，本地 Prompt 策略已独立开启。
- `full_text_context` 下提示：扫描更完整上下文，但日志仅保存脱敏预览。

## Runtime Behavior Matrix

本地策略只在现有风控总开关、内容审计开关、分组范围和模型范围全部通过后运行。具体组合行为如下：

| 总模式 / 关键词策略 | 本地规则状态 | 行为 |
|---|---|---|
| `off` 或内容审计 disabled | 任意 | 本地规则不运行，保持现有放行逻辑。 |
| 分组或模型不在审计范围 | 任意 | 本地规则不运行，保持现有跳过逻辑。 |
| `observe` | 命中 | 写 `local_rule_hit` 日志，不同步阻断；默认继续异步 OpenAI Moderations API，`skip_api_after_hit=true` 时跳过 API。 |
| `pre_block` + 关键词命中 | 任意 | 保持现有 `keyword_block` 优先阻断，本地规则不再重复扫描，避免同一请求写两类本地日志。 |
| `pre_block` + `keyword_only` + 关键词未命中 | 本地关闭或未命中 | 直接放行，不调用 OpenAI Moderations API。 |
| `pre_block` + `keyword_only` + 关键词未命中 | 本地命中 + `action=record` | 写 `local_rule_hit` 日志后放行，不调用 OpenAI Moderations API。 |
| `pre_block` + `keyword_only` + 关键词未命中 | 本地命中 + `action=block` | 写 `local_rule_block` 日志并阻断，不调用 OpenAI Moderations API。 |
| `pre_block` + `keyword_and_api` 或 `api_only` | 本地命中 + `action=record` + `skip_api_after_hit=false` | 写 `local_rule_hit` 日志，继续现有 OpenAI Moderations API 链路。 |
| `pre_block` + `keyword_and_api` 或 `api_only` | 本地命中 + `action=record` + `skip_api_after_hit=true` | 写 `local_rule_hit` 日志后放行，跳过 OpenAI Moderations API。 |
| `pre_block` + `keyword_and_api` 或 `api_only` | 本地命中 + `action=block` | 写 `local_rule_block` 日志并阻断，不调用 OpenAI Moderations API。 |
| 任意允许 API 的路径 | 本地未命中 | 继续现有风险哈希、抽样、API Key 检查和 OpenAI Moderations API 逻辑。 |

空输入规则：
- `latest_user_input` 下沿用当前 `ExtractContentModerationInput` 语义；没有可审计文本时保持现有 `skip_empty_input` 行为。
- `full_text_context` 下，即使最新用户输入为空，也应尝试从完整上下文抽取文本；抽取仍为空时才走 `skip_empty_input`。

风险哈希顺序：
- 关键词硬拦截仍优先于本地规则。
- 本地规则在风险哈希检查前运行，才能在无风险哈希命中时写本地规则日志。
- `record_hash=true` 时，本地规则使用扫描文本或最新输入的稳定 hash 写入现有风险哈希，不新增第二套 hash 系统。

## Failure Handling

实现必须把配置错误和运行时异常区分处理：

- 管理员保存配置时，自定义正则不可编译应返回配置错误，不写入 settings。
- 运行时如果 settings 被手工改坏导致规则编译失败，应记录 warn/error，跳过本地规则，继续现有风控链路；默认不能因本地规则引擎故障阻断用户请求。
- `full_text_context` JSON 解析失败时，不扫描原始 body 字符串，按空文本处理并继续现有链路。
- 抽取或扫描过程中遇到超长字段，必须先按 max length 头尾截断后再扫描。
- 日志写入失败只影响日志和副作用，不应改变已有请求决策规则；现有风控中心如果已有相同错误处理策略，应保持一致。
- 测试接口必须直接返回规则编译或配置校验错误，便于管理员修正。

## Migrations

新增 `backend/migrations/158_content_moderation_local_rules.sql`：

```sql
ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS local_rule_detail JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS exclude_from_auto_ban_count BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_content_moderation_logs_local_rule_action_created_at
  ON content_moderation_logs(action, created_at DESC)
  WHERE action IN ('local_rule_hit', 'local_rule_block');
```

迁移要求：
- 幂等。
- 不修改历史迁移。
- 历史日志 `exclude_from_auto_ban_count=false`，维持现有封禁计数语义。
- 回滚到旧二进制时，新增列会被忽略；旧二进制反序列化 settings 会忽略 `local_rules` 未知字段。

## Compatibility

旧配置：
- 没有 `local_rules` 字段时，本地策略默认关闭，行为与当前版本一致。

旧前端：
- 忽略新响应字段时不影响现有配置保存。
- 后端 update 应只覆盖传入字段，避免旧前端保存配置时清空本地规则。

旧日志：
- `local_rule_detail={}`。
- `exclude_from_auto_ban_count=false`。

现有 API Moderations：
- `base_url/model/api_keys/timeout/retry/worker/thresholds` 全部保持。
- `skip_api_after_hit` 只影响本地规则命中后的 API 联动。

现有关键词：
- `blocked_keywords` 和 `matched_keyword` 语义保持。
- `keyword_blocking_mode` 语义保持。

## Trade-Offs

- JSONB 日志详情比拆列查询弱，但规则详情扩展更低冲突，适合长期维护 fork。
- `exclude_from_auto_ban_count` 单独布尔列增加一列 schema，但能用稳定 SQL 排除不计封禁的本地命中，避免 action 组合越来越复杂。
- 本地规则默认不走 `sample_rate`，会增加少量 CPU，但它默认关闭，开启后正则引擎有长度限制、预编译缓存和 literal 预过滤。
- `full_text_context` 增加覆盖面，也增加误报风险；默认仍是 `latest_user_input`。

## Test Plan

后端单元测试：
- 默认配置本地规则关闭。
- 配置归一化默认值。
- 自定义 regex 非法时报错。
- 内置规则默认启用，disabled list 生效。
- 自定义规则 enabled=false 不参与命中。
- score/raw_score/strict_score/strict_hit。
- 防御语境降分。
- latest_user_input 复用现有抽取。
- full_text_context 覆盖 messages/system/instructions/input/prompt/style。
- full_text_context 跳过 url/file/data/b64_json/source/inline_data。
- 头尾截断保留尾部且 UTF-8 安全。
- 脱敏覆盖 Authorization、token/api_key/secret、cookie、sk、JWT、邮箱。

后端服务测试：
- 本地规则关闭时现有 `keyword_blocking_mode` 三模式行为不变。
- `pre_block + action=record` 命中后写 `local_rule_hit` 日志，请求放行。
- `pre_block + action=block` 命中后写 `local_rule_block` 日志，请求阻断，且不调用 Moderations API。
- `observe` 命中只记录，不阻断。
- `skip_api_after_hit=false` 默认继续调用现有 Moderations API。
- `skip_api_after_hit=true` 命中后跳过 Moderations API。
- `keyword_only` 下本地规则在 API short-circuit 前执行，但不重新打开 API。
- `api_only` 下关键词不生效，本地规则由自身开关控制。
- `count_for_auto_ban=false` 时 flagged 日志不被 `CountFlaggedByUserSince` 计入。
- `count_for_auto_ban=true` 时复用现有封禁阈值。
- `record_hash=false` 不写 Redis 风险哈希。
- `record_hash=true` 写 Redis 风险哈希。
- `email_on_hit=false` 不发送本地规则邮件。
- 测试接口不写日志、不调用 API、不触发副作用。

仓储测试：
- `CreateLog/ListLogs` 正确读写 `local_rule_detail`。
- `CountFlaggedByUserSince` 排除 `exclude_from_auto_ban_count=true`。
- `blocked` 结果筛选包含 `local_rule_block`。

前端测试：
- API 类型覆盖 local rules。
- 配置加载/保存不丢失 local rules。
- 内置规则禁用开关生成 `disabled_builtin_rules`。
- 测试工具展示命中规则和分数。
- 日志列表展示 local rule detail，不把本地规则展示成 matched keyword。

## Rollback

快速回滚：
- 设置 `content_moderation_config.local_rules.enabled=false`。

代码回滚：
- 旧二进制会忽略 settings 中的 `local_rules`。
- 旧二进制会忽略新增日志列。
- 新增迁移无需回滚；保留空 JSONB 和布尔列不影响旧逻辑。

风险缓解：
- 默认关闭本地规则。
- 首次开启默认只记录。
- 默认不封禁、不写哈希、不发邮件。
- 默认继续 OpenAI Moderations API，便于对比本地规则与现有审核结果。
