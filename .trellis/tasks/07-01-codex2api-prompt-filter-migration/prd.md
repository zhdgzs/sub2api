# 迁移 codex2api Prompt 本地审计策略到风控中心

## Goal

在当前项目的风控中心中新增一层可配置的“本地 Prompt 审计策略”，迁移 codex2api 的内置 cyber abuse 正则评分规则，使系统可以在不依赖 OpenAI Moderations API 的情况下先做本地命中、记录、拦截和封禁计数。

用户价值：
- 保留当前风控中心的 OpenAI Moderations 审核、前置拦截、异步观察、日志、哈希拦截、邮件通知和自动封禁能力。
- 增加 codex2api 已验证的一组 cyber abuse 本地策略，覆盖恶意软件、漏洞利用、凭证窃取、逆向破解、数据外泄等 Prompt 风险。
- 通过独立开关、模式、阈值和规则配置控制本地策略，允许先观察再拦截，降低误杀风险。
- 尽量减少对上游 sub2api 原系统结构的侵入，降低未来同步上游风控中心改动时的冲突范围。

## Confirmed Facts

- 当前项目已有风控中心，后端核心在 `backend/internal/service/content_moderation.go`，入口由各网关 handler 调用 `checkContentModeration`。
- 当前风控中心已有总开关 `risk_control_enabled` 和内容审计配置开关 `ContentModerationConfig.Enabled`。
- 当前风控中心支持 `off`、`observe`、`pre_block` 三种模式。
- 当前风控中心调用 OpenAI Moderations，默认模型为 `omni-moderation-latest`，并按分类阈值判定命中。
- 本 PRD 中“继续调用 API”或“继续调用 OpenAI Moderations API”均指继续走当前风控中心已经配置的审核接口，即现有 `base_url`、`model`、`api_key/api_keys`、超时、重试、worker 队列和阈值配置；默认模型仍是 `omni-moderation-latest`。
- 本 PRD 不新增 codex2api 的独立 review API 配置，不额外引入第二套 Moderations API Key、Base URL 或模型配置。
- 当前 UI 文案里的“API”或“上游审计接口”指风控中心服务端调用的 OpenAI Moderations API，不是 sub2api 风控中心自身的后台管理 REST API。
- 当前风控中心已有 `blocked_keywords` 与 `keyword_blocking_mode`，但只是大小写不敏感的字面包含匹配，不支持正则、权重、严格规则、分类和评分。
- 当前风控中心已有 `keyword_block`、`hash_block`、`block`、`cyber_policy` 等 action，并已有日志、邮件通知、自动封禁和 Redis 风险哈希能力。
- 当前风控中心的配置保存在 settings 的 `content_moderation_config` JSON 中，不是单独的配置表。
- 当前风控日志表 `content_moderation_logs` 已有 `matched_keyword` 字段，但没有记录本地规则列表、规则分数、raw score 或 strict hit 的结构化字段。
- codex2api 的本地 Prompt Filter 规则位于 `security/promptfilter/patterns.go`，规则结构包含 `name`、`pattern`、`weight`、`category`、`strict`。
- codex2api 默认本地规则阈值为 `50`，严格阈值为 `90`，默认最大扫描文本长度为 `80 KiB`。
- codex2api 的本地规则是正则评分系统，不是 OpenAI Moderations 分类阈值系统。
- codex2api 的 README 声明项目以 MIT License 发布，可以作为迁移参考。
- 已确认产品决策：本地策略命中后的“处理动作”和“副作用”拆成独立维度配置，而不是使用一个组合模式。
- 已确认产品决策：管理员开启本地策略总开关时，codex2api 内置规则默认全部启用；误报通过只记录模式、单规则禁用、阈值和副作用开关控制。
- 已确认产品决策：本地策略总开关从关闭切换为开启时，默认命中后动作是“只记录”；默认不计入自动封禁、不写风险哈希、不发送命中邮件，但这些副作用都必须可由管理员后续单独开启。
- 已确认产品决策：本地策略扫描范围必须可配置，支持 `latest_user_input` 和 `full_text_context`；默认使用 `latest_user_input`，降低误报并保持当前风控中心语义。
- 已确认产品决策：本地策略命中但未同步拦截时，是否继续调用 OpenAI Moderations API 必须作为独立配置项；默认继续调用，保持现有审核链路和日志完整性。
- 已确认产品决策：当前 `keyword_blocking_mode` 三种模式只作为“关键词列表与 API 的联动策略”保留，不直接改造成 codex2api 本地规则的总策略枚举。

## Requirements

- 必须把 codex2api 的内置 Prompt Filter 策略作为“本地审计策略层”接入当前风控中心，不替换现有 OpenAI Moderations 链路。
- 本地审计策略必须默认关闭，避免升级后改变现有用户请求行为。
- 本地审计策略必须受现有 `risk_control_enabled`、内容审计 `enabled`、模式、分组范围、模型范围约束，除非后续设计明确选择更强的全局策略。
- 本地审计策略应在输入提取完成后执行，并支持按配置选择扫描当前项目提取的最新用户输入或更完整的请求文本上下文。
- 扫描范围为 `latest_user_input` 时，必须复用当前 `ExtractContentModerationInput` 的协议适配能力，避免重复解析 Chat、Responses、Anthropic、Gemini、Images 请求体。
- 扫描范围为 `full_text_context` 时，必须实现接近 codex2api 的文本抽取语义：从请求内相关文本上下文中提取 `instructions`、`input`、`prompt`、`messages`、Anthropic `system/messages`、图片 `prompt/style` 等可读文本，但避免扫描图片 base64、文件内容、URL、大体积 data 字段等非目标文本。
- 本地策略扫描范围默认必须为 `latest_user_input`。
- 本地审计策略应在 OpenAI Moderations API 调用之前执行，以便低延迟发现明显风险，也能在无审核 API Key 时提供本地防护。
- 本地审计策略不得删除或改变原始请求内容，只能产生审计判定、日志和拦截结果。
- 必须尽量完整迁移 codex2api 内置规则的 `name`、`pattern`、`weight`、`category`、`strict` 语义。
- 管理员开启本地策略总开关时，codex2api 内置规则必须默认全部参与检测。
- 必须支持禁用单条内置规则，避免某些规则误报时只能关闭整套本地策略。
- 必须支持自定义本地规则，结构至少包含 `name`、`pattern`、`weight`、`category`、`strict`、`enabled`。
- 必须支持本地规则阈值配置：普通阈值、严格阈值、最大扫描长度。
- 必须支持本地规则命中后动作配置，至少覆盖：
  - 只记录：命中后写风控日志，不阻断请求。
  - 拦截请求：命中后在 `pre_block` 链路中阻断请求，并写风控日志。
- 本地策略配置必须拆成独立维度：命中后动作控制是否只记录或拦截；副作用控制是否计入自动封禁、是否写风险哈希、是否发送邮件。
- 命中后动作与副作用不得互相隐式绑定；例如允许“只记录但计入封禁”、也允许“拦截但不计入封禁”，具体行为由管理员配置决定。
- 当前后端 `persistContentModerationLog` 通过一个 `applySideEffects` 同时触发封禁计数和邮件通知；实现本地策略时必须解耦本地策略副作用，避免“计入封禁”和“发送命中邮件”被迫绑定。
- 本地策略首次开启时，命中后动作默认必须为“只记录”。
- 本地策略首次开启时，计入自动封禁、写风险哈希、发送命中邮件默认都必须为关闭状态。
- 如果当前风控中心处于 `observe` 模式，本地策略不得同步阻断请求；可以记录命中并按配置决定是否进入违规计数。
- 如果当前风控中心处于 `pre_block` 模式，本地策略命中且本地模式配置为拦截时，必须在调用 OpenAI Moderations API 前直接返回当前风控中心的拦截错误。
- 本地策略命中但未拦截时，后续是否继续调用当前风控中心已有的 OpenAI Moderations API 必须可控；默认继续调用，避免本地规则替代现有审核链路。
- 本地策略的“命中后是否继续调用 API”只影响 codex2api 本地规则命中后的 API 联动，不改变现有 `keyword_blocking_mode` 对关键词列表的语义。
- 本地策略新增的管理配置、日志展示和测试能力应复用或扩展当前 sub2api 风控中心后台 API；这里的“命中后是否继续调用 API”不得被理解为是否调用后台管理 API。
- 本地策略命中的日志必须进入现有 `content_moderation_logs`，使风控中心列表、筛选、邮件和封禁逻辑尽量复用。
- 日志必须能区分本地规则命中与关键词命中；不得把复杂规则简单塞入 `matched_keyword` 造成语义混乱。
- 应新增结构化日志字段或等价存储，用于记录命中规则列表、分数、raw score、strict hit 和命中分类。
- 本地策略日志的 action 应新增独立值：`local_rule_hit` 表示只记录命中，`local_rule_block` 表示同步拦截，避免和 `keyword_block`、OpenAI Moderations `block` 混淆。
- 本地策略必须支持脱敏预览，避免日志持久化 Authorization、API Key、token、cookie、JWT、邮箱等敏感信息。
- 本地策略的命中上下文最多记录有限片段，避免保存完整超长 Prompt。
- 本地策略的扫描文本必须做长度限制，并保留头尾扫描策略或等价机制，避免大请求导致性能或内存问题。
- `full_text_context` 模式也必须遵守扫描长度限制和脱敏日志要求，不得因为扫描范围扩大而持久化完整请求体。
- 正则规则必须在配置更新或服务启动后预编译并缓存，网关请求路径不得每次重复编译全部规则。
- 自定义正则保存前必须验证可编译；无效正则不得污染运行时配置。
- 规则引擎异常不得导致网关请求失败；异常时应记录错误并按安全策略放行或只跳过本地策略，默认不因规则引擎错误阻断请求。
- 如果 settings 被手工写入无效本地规则配置，运行时必须跳过本地规则并继续现有风控链路；管理员保存配置和测试接口仍必须返回明确错误。
- 本地策略必须支持测试接口，输入一段文本后返回命中规则、分数、strict hit、最终动作和脱敏预览，便于管理员调试阈值。
- 前端风控中心必须展示本地策略配置入口，包括总开关、处理模式、阈值、最大扫描长度、内置规则启停、自定义规则和测试工具。
- 前端风控日志必须展示本地规则命中详情，至少包括动作、最高分类、分数、命中规则摘要和输入预览。
- 本地策略与现有 `blocked_keywords` 必须共存；关键词适合管理员临时配置，codex2api 规则适合内置策略集。
- 本地策略不得改变 `blocked_keywords` 的已有行为、字段名和 API 契约。
- 本地策略不得改变 OpenAI Moderations 的分类阈值、API Key 管理、worker 队列、哈希拦截、邮件模板和自动封禁的既有语义，除非是明确接入命中日志后自然复用。
- 自动封禁必须复用当前 `auto_ban_enabled`、`ban_threshold`、`violation_window_hours` 逻辑；本地策略不得新建一套并行封禁系统。
- 必须支持配置本地策略命中是否计入自动封禁；默认关闭，允许管理员后续开启。
- 本地策略配置为不计入自动封禁时，相关日志即使 `flagged=true` 也不得被后续 `CountFlaggedByUserSince` 计入累计封禁次数。
- 必须支持配置本地策略命中是否记录风险哈希；默认关闭，允许管理员在观察误报率稳定后开启。
- 必须支持配置本地策略命中是否发送邮件，并复用当前风控中心邮件通知能力。
- 本地策略命中是否发送邮件默认关闭，允许管理员后续开启。
- 必须支持管理员关闭单条高误报内置规则后立即生效。
- 必须考虑未来上游 sub2api 也扩展风控中心时的冲突控制：新增能力应尽量集中在新文件、新字段、新 UI 区块和局部接入点，不大范围重写现有风控主流程。
- 数据库迁移必须向后兼容，使用 `ADD COLUMN IF NOT EXISTS` 等幂等写法，不破坏已有日志数据。
- 配置 JSON 必须向后兼容，旧配置缺少本地策略字段时应使用安全默认值。
- API 响应必须向后兼容，新增字段不应让旧前端或旧客户端失败。
- 后端配置更新必须向后兼容；旧前端未提交 `local_rules` 时不得清空已有本地策略配置。
- 不要求迁移 codex2api 调用 OpenAI Moderations 二次 review 的设计，因为当前项目已经有完整的 Moderations 审核链路。
- 不要求把 codex2api 的管理页面原样迁移；前端应按当前 Vue 风控中心的信息架构扩展。

## Relationship With Existing Keyword Strategy

当前项目已有的 `keyword_blocking_mode` 是“关键词列表与当前风控中心 OpenAI Moderations API 的联动策略”，不是风控总模式，也不是未来所有本地规则的总枚举。迁移 codex2api 本地规则时必须保留这个字段的既有含义，避免把新能力塞进旧枚举导致行为歧义和上游同步冲突。

本节中的 `API` 特指 OpenAI Moderations API，也就是当前前端文案里的“上游审计接口”。它不是风控中心页面调用的后台管理 API。后台管理 API 仍应沿用当前 sub2api 风控中心接口体系，只在现有配置、日志、测试接口上做兼容扩展。

现有三种模式的当前语义：
- `keyword_and_api`：关键词命中时直接拦截；关键词未命中时继续调用当前风控中心配置的 OpenAI Moderations API。
- `keyword_only`：只判断关键词；关键词未命中时直接放行，不调用当前风控中心配置的 OpenAI Moderations API。
- `api_only`：不判断关键词列表，只调用当前风控中心配置的 OpenAI Moderations API。

codex2api 本地规则与关键词的关系：
- 二者都属于本地检测，但不是同一种能力。
- 关键词是管理员配置的字面包含匹配，适合临时硬拦截。
- codex2api 本地规则是内置正则评分系统，支持权重、分类、strict、阈值、防御语境降分和自定义规则。
- 因此本地规则必须有自己的 `enabled`、`action`、`scan_scope`、阈值和副作用配置，而不是复用 `keyword_blocking_mode` 表达全部行为。

推荐执行顺序：
- 先完成风控总开关、内容审计开关、分组范围、模型范围、输入提取等现有前置判断。
- 在 `pre_block` 模式下，保持当前关键词硬拦截逻辑优先执行：如果 `keyword_blocking_mode != api_only` 且命中关键词，继续按现有 `keyword_block` 直接拦截。
- 关键词未命中或关键词未启用时，如果 codex2api 本地规则已开启，则执行本地规则扫描。
- 本地规则命中且配置为拦截，并且当前风控总模式允许同步拦截时，在调用 OpenAI Moderations API 前阻断。
- 本地规则命中但配置为只记录，或当前总模式为 `observe` 时，记录本地规则日志后再按 API 联动配置决定是否继续调用 OpenAI Moderations API。
- 最后再执行现有风险哈希和 OpenAI Moderations API 链路；具体接入点可在设计阶段根据当前代码最小侵入原则微调。

`keyword_blocking_mode` 与本地规则 API 联动的合成规则：
- `keyword_and_api`：API 默认在路径中。本地规则命中但未拦截时，默认继续调用 API；如果管理员开启“本地规则命中后跳过 API”，则跳过 API。
- `keyword_only`：API 本来就不在路径中。本地规则仍可在本地执行、记录或拦截，但命中后的“继续调用 API”配置不生效，避免破坏“仅关键词/低 API 用量”的既有成本语义。
- `api_only`：关键词列表不生效。codex2api 本地规则是否运行由本地规则自己的总开关决定；如果本地规则关闭，则行为就是纯 API；如果本地规则开启，则表示管理员显式启用了“本地规则 + API”的额外前置检测。

前端文案要求：
- 当前 UI 中 `keyword_blocking_mode` 不应继续只叫“审计策略”，迁移后应改为“关键词/API 策略”或“关键词审计策略”，避免管理员误以为它控制 codex2api 本地规则。
- codex2api 本地规则配置区应单独展示“命中后 API 行为”，例如：`继续调用 OpenAI Moderations API` / `跳过 OpenAI Moderations API`。
- 在 `keyword_only` 下，应提示“当前策略不调用 API，本地规则命中后的 API 行为配置不会生效”。
- 在 `api_only` 下，如果本地规则开启，应提示“仅 API 只表示关键词列表不生效；本地 Prompt 策略已独立开启”。

## Full Text Context Semantics

`full_text_context` 是扫描范围配置中的增强模式，目标是接近 codex2api `security/promptfilter/filter.go` 的 `ExtractText`、`collectGJSONText`、`limitScanText` 行为，但仍保持当前项目日志脱敏和兼容性要求。

核心原则：
- 不扫描完整原始 JSON 字符串，而是解析请求体后只抽取“模型会看到的可读文本”。
- 不抽取二进制、base64、文件、URL、图片引用、结果数据等高噪声或高体积字段。
- 抽取出的文本只用于本地规则扫描；日志仍只能保存脱敏预览和有限命中片段，不得持久化完整请求体。
- 扫描结果必须经过最大长度限制，避免超长请求影响网关路径性能。

endpoint 字段映射需要参考 codex2api：
- Chat Completions：`chat`、`chat_completions`、`/v1/chat/completions` 只抽取 `messages`。
- Anthropic Messages：`messages`、`anthropic`、`/v1/messages` 抽取 `system` 和 `messages`。
- Images：`image`、`images`、`images_generations`、`images_edits`、`/v1/images/generations`、`/v1/images/edits` 抽取 `prompt` 和 `style`。
- 默认路径，包含 Responses 类请求：抽取 `instructions`、`input`、`prompt`、`messages`。

递归抽取规则需要参考 codex2api：
- 数组：递归处理每个元素。
- 对象：优先读取 `text` 字符串字段。
- 对象：读取 `content` 字段；如果是字符串则直接加入，如果是数组或对象则继续递归。
- 对象：遍历其他字段继续递归，但必须跳过非目标字段。
- 字符串：去掉首尾空白后加入扫描文本。
- 空值、布尔值、数字、不可解析 JSON 不参与扫描。

必须跳过的字段名，大小写不敏感，并尽量与 codex2api 保持一致：
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

跳过字段的原因：
- `text`、`content` 已经被对象优先逻辑处理，遍历时跳过是为了避免重复计分。
- `image_url`、`url`、`file_id`、`source`、`file` 通常是引用、文件句柄或外部资源，不应展开扫描。
- `data`、`b64_json` 容易包含 base64、二进制或大体积数据，扫描成本高且误报风险高。
- `type`、`role` 是结构元数据，不是用户意图文本。
- `result` 常见于工具或响应结果，当前迁移阶段不把它纳入 Prompt 风险扫描。

长度限制需要参考 codex2api：
- 默认最大扫描长度为 `80 KiB`，并允许管理员配置。
- 当抽取文本不超过最大长度时，完整扫描抽取文本。
- 当抽取文本超过最大长度时，保留头部和尾部组合扫描，默认接近 `64 KiB` 头部 + `16 KiB` 尾部。
- 如果管理员把最大长度配置得小于头尾默认总和，则按约 `4/5` 头部、`1/5` 尾部分配。
- 截断必须保证 UTF-8 安全，不得切坏多字节字符。

与 `latest_user_input` 的差异：
- `latest_user_input` 更接近当前项目已有语义，只扫描当前协议中提取出的最新用户输入，默认使用它以降低误报和行为变化。
- `full_text_context` 会扫描更完整的会话上下文，例如 system、instructions、历史 messages、嵌套 content/text 字段，覆盖面更接近 codex2api。
- `full_text_context` 更容易发现隐藏在历史消息或复杂结构里的风险文本，但也更容易因为历史上下文、引用材料或防御分析内容触发规则，因此必须仍受阈值、防御语境降分、只记录模式和副作用开关保护。

## Suggested Technical Shape

- 新增本地规则引擎文件，优先放在 `backend/internal/service/content_moderation_local_rules.go`，避免把 `content_moderation.go` 继续膨胀。
- 新增内置规则数据文件，优先放在 `backend/internal/service/content_moderation_local_rule_patterns.go` 或独立 package，减少与主服务冲突。
- 在 `ContentModerationConfig` 中新增嵌套配置，例如 `LocalRules ContentModerationLocalRuleConfig`，避免散落多个顶层字段。
- 在 `ContentModerationConfigView`、`UpdateContentModerationConfigInput`、admin handler、frontend API 类型中透传同一嵌套配置。
- 在 `ContentModerationLog` 中新增本地规则详情字段，仓储层负责 JSONB marshal/unmarshal。
- 在 `Check` 流程中新增一个小接入点函数，例如 `s.checkLocalRules(...)`，保持主流程可读。
- 本地规则命中结果转换为现有 `ContentModerationDecision` 和 `ContentModerationLog`，最大化复用现有日志、邮件、封禁和哈希逻辑。

## Acceptance Criteria

- [ ] 旧配置中不存在本地策略字段时，风控中心行为与当前版本一致。
- [ ] 本地策略默认关闭，升级后不会新增拦截、日志或封禁。
- [ ] 开启本地策略且处于只记录模式时，命中 codex2api 内置规则会写入风控日志，但请求继续放行。
- [ ] 本地策略首次开启后的默认行为是只记录，不拦截、不计入自动封禁、不写风险哈希、不发送命中邮件。
- [ ] 开启本地策略且处于 `pre_block` + 拦截模式时，命中达到阈值会在 OpenAI Moderations API 调用前阻断请求。
- [ ] 本地策略命中但未拦截时，默认继续调用 OpenAI Moderations API。
- [ ] 管理员可以配置本地策略命中但未拦截时跳过 OpenAI Moderations API。
- [ ] 现有 `keyword_blocking_mode` 的三种行为在本地策略关闭时完全保持不变。
- [ ] `keyword_only` 下本地规则可以执行本地记录或拦截，但不会重新打开 OpenAI Moderations API 调用。
- [ ] `api_only` 下关键词列表不生效；本地规则是否运行只由本地规则总开关决定。
- [ ] 当前风控中心处于 `observe` 模式时，本地策略命中不会同步阻断请求。
- [ ] 当前风控中心处于 `off` 或内容审计 disabled 时，本地策略不会运行。
- [ ] 本地策略遵守现有分组范围配置。
- [ ] 本地策略遵守现有模型范围配置。
- [ ] 本地策略扫描范围可配置为 `latest_user_input` 或 `full_text_context`。
- [ ] 本地策略默认扫描范围为 `latest_user_input`。
- [ ] `latest_user_input` 模式复用当前 `ExtractContentModerationInput` 的最新用户输入语义。
- [ ] `full_text_context` 模式覆盖 codex2api 风格的完整文本上下文抽取，但跳过 base64、文件内容、URL 和大体积 data 字段。
- [ ] `full_text_context` 下最新用户输入为空时仍会尝试扫描完整文本上下文；完整上下文也为空时才按空输入跳过。
- [ ] codex2api 内置规则的名称、分类、权重、strict 标记和正则语义被迁移并可被测试接口返回。
- [ ] 本地策略开启后，未被显式禁用的全部内置规则默认参与检测。
- [ ] 管理员可以禁用任意单条内置规则，禁用后该规则不再命中。
- [ ] 管理员可以新增自定义正则规则，保存前校验正则合法性。
- [ ] 自定义规则可以被禁用，禁用后不参与评分。
- [ ] 本地策略普通阈值和严格阈值可配置，修改后无需重启即可生效。
- [ ] 达到普通阈值但未达到严格阈值时，按普通命中逻辑处理。
- [ ] 严格规则累计分达到严格阈值时，即使普通分数逻辑变化也能判定命中。
- [ ] 防御语境降分或等价机制被保留，避免“如何防御/检测/修复”类 Prompt 大量误报。
- [ ] 日志中能查看命中规则列表、分数、raw score、strict hit、最高分类和脱敏输入预览。
- [ ] 日志能区分 `keyword_block` 与本地规则命中。
- [ ] 本地规则命中是否计入自动封禁可配置。
- [ ] 当配置为计入自动封禁时，本地规则命中复用现有 `ban_threshold` 和 `violation_window_hours`。
- [ ] 当配置为不计入自动封禁时，本地规则命中仍可记录日志但不增加封禁次数。
- [ ] 当配置为不计入自动封禁时，该类本地规则日志不会被历史累计查询计入后续封禁次数。
- [ ] 本地规则命中是否写入风险哈希可配置。
- [ ] 本地规则命中是否发送邮件可配置。
- [ ] 前端配置区把命中后动作、副作用开关分开展示，避免使用含义不清的组合模式。
- [ ] 敏感字段脱敏覆盖 Authorization、password/token/api_key/secret、cookie、OpenAI sk key、JWT、邮箱。
- [ ] 超长输入不会导致请求路径正则扫描超过配置长度。
- [ ] 正则编译错误不会导致服务启动失败或网关请求失败；无效自定义规则在保存时被拒绝。
- [ ] 旧前端保存其他风控配置时不会删除或重置已有 `local_rules`。
- [ ] 本地规则运行时异常会记录并跳过本地规则，不改变现有 OpenAI Moderations 链路。
- [ ] 单元测试覆盖规则评分、strict hit、禁用规则、自定义规则、防御语境降分、脱敏和超长输入截断。
- [ ] 集成测试覆盖 `pre_block` 拦截、`observe` 只记录、禁用状态不运行、分组/模型范围跳过、封禁计数开关。
- [ ] 前端风控中心可以配置本地策略、查看内置规则、编辑自定义规则并测试输入。

## Out of Scope

- 不替换当前 OpenAI Moderations 审核链路。
- 不迁移 codex2api 的 React 管理页面。
- 不改变现有 `blocked_keywords` 字段语义。
- 不新增独立于风控中心的封禁系统。
- 不改造用户错误、用量统计、账号调度或上游 cyber_policy 透传逻辑。
- 不在本任务中调整 codex2api 内置规则本身的安全策略边界；规则调优应作为后续独立任务处理。
- 不要求历史日志回填本地规则字段。
- 不要求一次性支持多套规则集版本管理。

## Open Questions

- 暂无。

## Design Completion Checklist

- [x] 默认值已确定：本地策略关闭；开启后只记录；内置规则全启用；副作用全关闭；扫描 `latest_user_input`；命中后继续 API。
- [x] 已明确 `keyword_blocking_mode` 与本地策略的关系：旧字段只控制关键词/API，本地策略独立配置。
- [x] 已明确 `full_text_context` 语义：接近 codex2api 抽取，但跳过 base64、文件、URL、大体积 data 字段。
- [x] 已明确副作用维度：自动封禁、风险哈希、邮件通知互不绑定。
- [x] 已明确日志模型：新增 `local_rule_detail` 和 `exclude_from_auto_ban_count`，新增 action。
- [x] 已明确兼容策略：旧配置、旧前端、旧日志、旧二进制回滚均不破坏。
- [x] 已明确不确定问题：当前无阻塞产品问题，后续进入实现和验证即可。
