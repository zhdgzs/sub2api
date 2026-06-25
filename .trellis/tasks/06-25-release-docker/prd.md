# 发布 Docker 镜像

## Goal

将当前 `custom` 分支上的“账号列表内直接修改优先级”改动提交到仓库，并按项目正式发布流程构建新的 Docker release 镜像与 release tag。

## Requirements

- 仅在 `/root/sub2api` 仓库内执行。
- 当前待提交改动必须先以中文提交信息提交到 `custom` 分支。
- 发布流程必须遵守 `docs/SYNC_UPSTREAM_CN.md` 和 `release-docker` skill 约束。
- 发布前必须执行只读预检，确认源分支、版本候选、release tag 和 Docker tag。
- 正式执行前必须向用户确认版本号。
- 不自动 stash、不中途自动解决冲突、不推送到 `upstream`。
- 默认保留本地 Docker 构建缓存，不使用 `--no-cache`，不做 prune。

## Acceptance Criteria

- [ ] 当前 4 个前端文件已提交到 `custom` 分支，提交信息为简体中文。
- [ ] `release_docker.py --repo "$PWD"` 预检已运行并输出候选版本、release tag、Docker tags。
- [ ] 用户已确认最终版本号。
- [ ] `release_docker.py --repo "$PWD" --yes --version <confirmed-version>` 已成功执行，或在冲突/校验失败处明确中止并汇报原因。
- [ ] 最终结果包含：版本号、release tag、对应 commit、Docker tags、是否存在警告。

## Confirmed Facts

- 当前仓库路径是 `/root/sub2api`。
- 当前分支是 `custom`。
- 工作区包含 4 个未提交文件，内容是账号管理列表内直接修改优先级的前端改动。
- 用户已经同意创建 Trellis 任务，并要求“提交代码，然后继续打包 docker”。

## Out Of Scope

- 不修改发布脚本逻辑。
- 不额外清理 Docker 缓存或镜像。
- 不执行与本次发布无关的功能开发或代码重构。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
