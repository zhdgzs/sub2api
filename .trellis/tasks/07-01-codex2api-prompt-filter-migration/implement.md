# 迁移 codex2api Prompt 本地审计策略到风控中心 - Implementation Plan

## Preconditions

- 当前开发分支必须是 `feature/codex2api-prompt-intercept-rules` 或其他 `feature/*` 分支。
- 实现前读取：
  - `.trellis/tasks/07-01-codex2api-prompt-filter-migration/prd.md`
  - `.trellis/tasks/07-01-codex2api-prompt-filter-migration/design.md`
  - `.trellis/spec/backend/index.md`
  - `.trellis/spec/frontend/index.md`
  - `.trellis/spec/guides/index.md`
- 任何提交前读取并遵守 `docs/SYNC_UPSTREAM_CN.md`。

## Execution Order

1. 后端配置与类型
   - 在 `ContentModerationConfig`、view、update input 中新增 `LocalRules`。
   - 增加 local rules 默认值、归一化、校验、clone。
   - handler request 透传 `local_rules`。
   - 保证旧前端不传 `local_rules` 时不清空已有配置。

2. 本地规则引擎
   - 新增 codex2api 内置规则数据文件。
   - 新增规则引擎、正则预编译缓存、评分、防御语境降分。
   - 规则引擎运行时异常时跳过本地规则并记录错误，不阻断请求。

3. 扫描文本抽取
   - `latest_user_input` 复用现有 `ExtractContentModerationInput`。
   - 新增 `full_text_context` 抽取，参考 codex2api 并跳过 base64、文件、URL、大体积 data 字段。
   - 支持头尾截断和 UTF-8 安全截断。
   - `full_text_context` 下最新用户输入为空时仍尝试扫描完整上下文。

4. 测试接口
   - 新增测试接口服务方法，不触发日志/API/副作用。
   - 测试接口返回命中规则、分数、strict hit、动作和脱敏预览。

5. 日志与副作用
   - 新增迁移 `158_content_moderation_local_rules.sql`。
   - 扩展 `ContentModerationLog`、repository create/list/count。
   - 把 `persistContentModerationLog` 副作用从单一 bool 拆成 policy。
   - 保持现有关键词、hash、Moderations API 命中行为等价。

6. 主流程接入
   - 在关键词硬拦截之后、`keyword_only` 放行之前接入本地规则。
   - 支持 `local_rule_hit`、`local_rule_block`。
   - 支持 `skip_api_after_hit` 与 `keyword_blocking_mode` 合成语义。
   - 保持 `off`、disabled、分组范围、模型范围不运行本地规则。

7. 后台 API
   - 新增 `POST /admin/risk-control/local-rules/test`。
   - 扩展 config/logs 响应。

8. 前端
   - 扩展 `riskControl.ts` 类型和测试 API。
   - 风控设置新增本地 Prompt 策略 tab。
   - 关键词 tab 文案改为“关键词/API 策略”。
   - 日志列表展示本地规则详情。
   - i18n 增加中英文文案。

9. 测试
   - 后端 unit tests 覆盖规则评分、抽取、脱敏、配置、主流程和仓储。
   - 前端 tests 覆盖配置加载/保存关键路径。

## Module Gates

每个模块完成后应满足对应 gate，再进入下一块，避免把问题堆到最后：

- 配置 gate：旧配置反序列化后 `local_rules.enabled=false`；旧前端 update 不清空 `local_rules`。
- 引擎 gate：内置规则命中、自定义规则命中、禁用规则、strict hit、防御语境降分均有单元测试。
- 抽取 gate：`latest_user_input`、`full_text_context`、跳过字段、超长截断均有单元测试。
- 日志 gate：`local_rule_detail` 可读写，`exclude_from_auto_ban_count=true` 不被封禁累计查询计入。
- 主流程 gate：`keyword_only`、`api_only`、`keyword_and_api` 与本地规则组合行为符合 design 矩阵。
- 前端 gate：配置保存不丢字段，测试工具可调用，日志可读本地规则详情。

## Validation Commands

优先运行聚焦测试：

```bash
go test ./backend/internal/service -run 'ContentModeration|LocalRule'
go test ./backend/internal/repository -run 'ContentModeration'
go test ./backend/internal/handler/admin -run 'ContentModeration'
```

前端聚焦测试：

```bash
cd frontend && pnpm test -- RiskControlView
```

收尾验证根据耗时选择：

```bash
go test ./backend/internal/service ./backend/internal/repository ./backend/internal/handler/admin
cd frontend && pnpm type-check
```

## Rollback Points

- 若规则引擎导致热路径风险，先保留配置和 UI，强制 `local_rules.enabled=false`。
- 若副作用拆分风险较高，先保留现有 API/关键词 policy 等价测试，再接本地规则 policy。
- 若前端 UI 复杂度过高，先提供基础配置和测试工具，内置规则表可简化为列表开关。

## Known Environment Risk

- 当前工作环境可能没有 `go` / `gofmt` 可执行文件；实现验证阶段必须先检查 `command -v go gofmt`。
- 如果工具链不可用，至少完成静态审查、前端 type-check 可用性检查，并在最终结果中明确说明 Go 测试未运行原因。

## Commit Plan

最终提交前：
- `git status --short --branch`
- 运行聚焦后端测试和前端 type-check。
- 提交信息使用中文，例如：`feat(risk): 迁移 codex2api 本地 Prompt 审计策略`
