# Git 操作规范与 cust 维护流程

本仓库是 `Wei-Shaw/sub2api` 的 fork。长期维护目标：

- `main` 只作为上游镜像。
- `cust` 是唯一长期定制、日常部署和正式 Docker 发布分支。
- `feature/*` 从 `cust` 创建，完成后合回 `cust`。
- 历史 `custom` 分支保留原样，不再同步、合并、发布或删除。

## Agent Git 操作约束

执行 `git commit` 前：

- 确认当前分支，禁止把自定义功能提交到 `main`。
- 检查 `git status --short --branch`，避免混入无关改动。
- 提交信息必须使用简体中文；Conventional Commits 前缀可以保留英文，例如 `feat(admin): 增加账号行内导出`。
- 数据库迁移应与业务逻辑分开提交。

执行 `git pull` 前：

- `main` 只能从 `origin/main` 或 `upstream/main` 更新。
- `cust` 只能从 `origin/cust` 更新。
- 工作树不干净时不得直接 pull。

执行 `git push` 前：

- 只允许推送到 `origin`，禁止推送到 `upstream`。
- `main` 只推送上游镜像更新。
- 自定义功能只推送到 `cust` 或 `feature/*`。
- 禁止 force-push，除非用户对具体目标另行明确授权。

通用要求：

- 不回滚用户未要求回滚的改动。
- 不使用 `git reset --hard` 或 `git checkout -- <file>` 等破坏性命令。
- 新增前端依赖时同步更新 `frontend/pnpm-lock.yaml`。
- 适合回馈上游的功能从 `main` 单独创建 PR 分支，不从 `cust` 提交上游 PR。

## 分支职责

- `main`：上游镜像，尽量与 `upstream/main` 一致。
- `cust`：唯一长期定制和正式发布分支。
- `feature/<name>`：单功能开发分支，从 `cust` 创建并合回 `cust`。
- `custom`：历史分支，仅保留，不处理。

`origin` 是自己的 fork；`upstream` 必须指向 `https://github.com/Wei-Shaw/sub2api.git` 且禁用 push URL。

## 初始化 cust

首次建立 `cust` 时从最新官方 `main` 创建：

```bash
git fetch upstream main
git switch -c cust upstream/main
```

首次正式发布时若 `origin/cust` 不存在，发布脚本使用 `git push -u origin cust` 创建远端分支。此动作只能由用户明确调用发布 skill 后执行。

## 开发功能

```bash
git switch cust
git pull --ff-only origin cust
git switch -c feature/my-feature
```

完成并验证后再合回 `cust`。除非用户明确要求，否则 agent 不主动 commit、merge 或 push。

## 同步上游

上游更新拆成两步，不能混为一个未经授权的流程：

1. `upstream/main -> origin/main -> local main`
2. `main -> cust`

`sync-upstream` 只负责第一步，并在当前非 `main` 分支上做只读冲突预检，不会合入 `cust`。

```bash
python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD"
```

用户确认后才可以执行：

```bash
python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD" --yes
```

将 `main` 合入 `cust` 是单独操作：

```bash
git switch cust
git pull --ff-only origin cust
git merge --no-ff main -m "chore: 合并上游 main <version>"
```

合并后至少运行：

```bash
make test-backend
make test-frontend
make build
```

## Docker 发布

正式镜像只从 `cust` 构建。远程发布是默认方式：

```text
$remote-docker
```

直接调用该 skill 视为授权其完整发布流程：同步上游、风险预检、切换和更新 `cust`、必要的合并与中文提交、推送 `origin/cust`、触发 GitHub Actions，并启动外部部署 watcher。

远程发布的统一契约：

- workflow：`.github/workflows/remote-docker.yml`
- 触发分支：`cust`
- checkout 分支：`cust`
- 正式版本：`backend/cmd/server/VERSION`
- 发布记录：`docs/DOCKER_RELEASE_HISTORY.md`
- 镜像：`ghcr.io/<owner>/sub2api:<version>`
- 滚动镜像：`ghcr.io/<owner>/sub2api:cust`
- watcher 参数：`BRANCH=cust`、`REMOTE_IMAGE=...:cust`、`LOCAL_IMAGE=sub2api:cust`

发布 preview 必须只读；发现文本冲突或高风险共享修改时，必须汇总风险并等待用户一次性决策。生成文件必须先解决源定义再重新生成，不能直接选边。i18n 文件必须检查对象键唯一性。

远程发布不创建 release tag。本地 `release-docker` 仅在用户明确要求本地正式构建时使用，并可能创建 release tag。

## 版本规则

版本格式为上游版本加 fork 后缀：

```text
0.1.172-zhdgzs.1
0.1.172-zhdgzs.2
```

每次正式发布更新：

- `backend/cmd/server/VERSION`
- `docs/DOCKER_RELEASE_HISTORY.md`

发布记录只统计 `cust` 相对 `main` 的自定义提交，不统计上游提交。发布提交使用中文，例如：

```text
chore(release): 发布 0.1.172-zhdgzs.1
```

## 发布检查清单

- [ ] 当前发布目标是 `cust`。
- [ ] 工作树干净。
- [ ] `main` 已同步所需的最新上游版本。
- [ ] 合并风险预检已通过或已按用户决策处理。
- [ ] 前后端测试与构建通过。
- [ ] VERSION 与发布记录一致。
- [ ] workflow 的 trigger、checkout、脚本 push、镜像标签和 watcher 参数均为 `cust`。
- [ ] 推送的是 `origin/cust`，没有修改或推送 `custom`。
