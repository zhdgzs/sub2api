# Git 操作规范与 Fork 维护流程

本仓库是 `Wei-Shaw/sub2api` 的 fork。长期维护目标是：`main` 只作为上游镜像，`custom` 是唯一日常部署和 Docker 打包分支，单个功能在 `feature/<name>` 中开发，发布版本用 `release/*` tag 固定。

本文件集中维护本仓库的 Git 操作约束、分支模型、上游同步流程、发布流程和长期定制维护规则。任何 agent 在执行或建议 `git commit`、`git push`、`git pull`，或处理上游同步、分支合并、发布、长期定制维护前，必须先阅读本文件。

## Agent Git 操作约束

执行 `git commit` 前：

- 先确认当前分支，避免把自定义改动提交到 `main`。
- 如果当前分支是 `main`，只允许上游同步类提交；其他改动应切到 `custom` 或 `feature/<name>`。
- 检查 `git status --short --branch`，确认没有混入无关改动。
- 涉及数据库 schema 或迁移的改动应单独提交。
- 提交信息必须使用简体中文描述变更；如沿用 Conventional Commits，`type(scope):` 可保留英文，例如 `fix(admin): 修复 OpenAI 重新授权套餐同步`。

执行 `git pull` 前：

- 先确认当前分支和远端跟踪关系。
- `main` 只能从 `origin/main` 或 `upstream/main` 更新。
- `custom` 只能从 `origin/custom` 更新。
- 不要在有未提交改动时直接 pull，除非用户明确要求并已说明冲突风险。

执行 `git push` 前：

- 先确认推送目标是 `origin`。
- 不要推送到 `upstream`。
- `main` 只推送上游镜像更新。
- 自定义功能只能推送到 `custom` 或 `feature/*`。
- 发布 tag 只推送到 `origin` 的 `release/*` tag，不能推送到 `upstream`。

通用变更卫生：

- 不要回滚用户未要求回滚的改动。
- 不要使用破坏性 Git 命令，例如 `git reset --hard` 或 `git checkout -- <file>`，除非用户明确要求。
- 新增前端依赖时，必须同步提交 `frontend/pnpm-lock.yaml`。
- 适合回馈上游的功能应从 `main` 单独拉 PR 分支，不要直接拿 `custom` 提上游 PR。

## 分支职责

严格按下面的职责使用分支：

- `main`: 上游镜像分支，永远尽量接近 `upstream/main`，不提交自己的功能、配置、文档或部署改动。
- `custom`: 长期定制和唯一日常部署分支，所有正式 Docker 镜像都从这里构建，所有自定义功能最终合并到这里。
- `feature/<name>`: 单个功能开发分支，从 `custom` 拉出，完成后合并回 `custom`。
- `release/v<version>-zhdgzs.<n>`: 发布 tag，不是常规分支。tag 必须指向 `custom` 上已验证、已更新版本号的 commit。

可选临时分支：

- `feature/upstream-v<version>`: 仅在上游更新很大、涉及迁移、或冲突风险高时，用于预演 `main` 合并进 `custom`；验证后合并回 `custom` 并删除。
- `release/<version>` 分支：默认不创建。只有需要长期冻结、灰度或回滚排查时，才从对应 `release/*` tag 临时创建。
- `sync/*`: 不再作为常规分支类型。只有历史兼容、复现旧同步流程、或特殊救援场景才临时使用；不能作为 Docker 或 release 来源。

核心规则：`main` 只负责接收上游，`custom` 才是自己的产品线和 Docker 打包来源。

## 仓库关系

- `origin`: `git@github.com:zhdgzs/sub2api.git`，自己的 fork，用于推送 `main`、`custom` 和 `feature/*`。
- `upstream`: `https://github.com/Wei-Shaw/sub2api.git`，原始仓库，只用于拉取上游更新。

建议禁用 `upstream` 的 push URL，避免误推到原仓库。

## 一次性本地配置

如果换了新机器或重新 clone 仓库，先执行：

```bash
git remote add upstream https://github.com/Wei-Shaw/sub2api.git
git remote set-url --push upstream DISABLED
git config rerere.enabled true
git fetch upstream --tags
```

如果 `upstream` 已存在，只需要确认：

```bash
git remote -v
git config --get rerere.enabled
```

说明：

- `upstream` 只用于拉取上游代码。
- `rerere.enabled=true` 会记录冲突解决方式，后续类似冲突可自动复用。

## 初始化 custom 分支

第一次建立自己的长期定制分支时：

```bash
git checkout main
git fetch upstream --tags
git merge --ff-only upstream/main
git push origin main

git checkout -b custom
git push -u origin custom
```

如果 `git merge --ff-only upstream/main` 失败，说明 `main` 已经有不属于上游的提交。不要强行 merge，先把自定义提交迁移到 `custom`，再恢复 `main` 的上游镜像职责。

## 开发自定义功能

所有自定义功能都从 `custom` 拉分支：

```bash
git checkout custom
git pull --ff-only origin custom

git checkout -b feature/my-feature
# 开发并提交
git push origin feature/my-feature
```

功能完成后合并回 `custom`，不要合并到 `main`：

```bash
git checkout custom
git pull --ff-only origin custom
git merge --no-ff feature/my-feature
git push origin custom
```

`feature/*` 可以推送到 `origin/feature/*` 用于备份、跨机器开发或代码审查，但不能作为正式 Docker 或 release 来源。小型临时改动可以直接在 `custom` 修改并提交，但应避免让 `custom` 长期处于脏工作区。

## 同步上游新版

上游发布新版后，流程拆成两个独立步骤：

1. 同步上游镜像分支 `main`。
2. 将最新 `main` 合入部署分支 `custom`。

这两个步骤不要混在同一个自动流程里。`sync-upstream` 只负责第一步；如果当前不在 `main`，它最多做虚拟冲突预检，不会修改当前分支。

第一步：更新上游镜像分支 `main`。

```bash
git fetch upstream --tags

git checkout main
git merge --ff-only upstream/main
git push origin main
```

第二步：把上游变更合并进自己的长期定制分支。

```bash
git checkout custom
git pull --ff-only origin custom
git merge --no-ff main -m "chore: merge upstream main v0.1.134"
```

如果发生冲突：

```bash
git status
# 手工解决冲突文件
git add <resolved-files>
git commit
```

如果上游改动很大、涉及迁移、或预检有冲突，可以先创建临时验证分支：

```bash
git checkout custom
git pull --ff-only origin custom
git checkout -b feature/upstream-v0.1.134
git merge --no-ff main
```

验证通过后再将该临时分支合并回 `custom` 并删除。常规流程不再创建 `sync/*`。

合并完成后执行验证：

```bash
make test-backend
make test-frontend
make build
```

验证通过后推送 `custom`：

```bash
git push origin custom
```

## 只合并上游 tag 的场景

默认推荐让 `main` 跟随 `upstream/main`，因为上游 release 后通常还会有版本文件同步提交。

如果你明确只想基于某个 release tag，例如 `v0.1.133`：

```bash
git fetch upstream --tags
git checkout main
git merge --ff-only v0.1.133
git push origin main
```

随后仍然按同样方式把 `main` 合并进 `custom`。

## 发布、版本号和 Docker 打包

正式 Docker 镜像只从 `custom` 构建。`main`、`feature/*`、`sync/*` 都不能作为正式 Docker 或 release 来源。

发布前要求：

- 当前分支必须是 `custom`。
- `git status --short --branch` 必须干净。
- `custom` 必须包含最新需要发布的 `main`。
- 版本号只在 `custom` 上更新。
- 版本号 commit 必须紧贴 release tag。
- 正式发布前，当前 commit 必须已推送到 `origin/custom`。

版本号文件：

```text
backend/cmd/server/VERSION
```

Docker 打包记录文件：

```text
docs/DOCKER_RELEASE_HISTORY.md
```

每次正式 Docker 打包都要补一条一句话功能记录，只统计本仓库自定义提交，不统计 `main` / `upstream` 的提交。

版本命名使用上游版本号加 fork 后缀：

```text
0.1.133-zhdgzs.1
0.1.133-zhdgzs.2
0.1.134-zhdgzs.1
```

发布流程：

```bash
git checkout custom
git pull --ff-only origin custom

# 按需更新 backend/cmd/server/VERSION，例如写入 0.1.134-zhdgzs.1
python3 "scripts/update_docker_release_record.py" --repo "$PWD" --version 0.1.134-zhdgzs.1

# 同时提交 docs/DOCKER_RELEASE_HISTORY.md，记录本次打包功能
git add backend/cmd/server/VERSION docs/DOCKER_RELEASE_HISTORY.md
git commit -m "chore(release): 0.1.134-zhdgzs.1"
git push origin custom

git tag -a release/v0.1.134-zhdgzs.1 -m "release/v0.1.134-zhdgzs.1"
git push origin release/v0.1.134-zhdgzs.1
```

Docker tag 不使用 `release/*`，因为 Docker tag 不能包含 `/`。正式镜像 tag 与 release tag 去掉 `release/v` 后的版本号对齐：

```bash
docker build \
  --build-arg VERSION=0.1.134-zhdgzs.1 \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  -t sub2api:0.1.134-zhdgzs.1 \
  -t sub2api:custom \
  .
```

本地测试镜像例外：可以在未推送前构建，但只能使用 `sub2api:local` 或 `sub2api:dev`，不能使用正式版本号 tag。

如果 release tag 打错，不默认 force 修改。应先明确记录错误，必要时删除本地和远端 tag 后重打；删除/重打 tag 属于高风险操作，必须单独确认。

## 冲突控制原则

长期维护 fork 的关键是减少 `custom` 和上游核心代码的重叠修改：

- 优先新增配置项、独立 service、独立 handler、独立前端组件。
- 避免大面积重写上游调度、鉴权、计费、迁移等核心路径。
- 必须改核心逻辑时，把改动拆成小 commit，并写清楚原因。
- 新增依赖时，前端必须同步提交 `frontend/pnpm-lock.yaml`。
- 数据库 schema 或迁移相关改动要单独提交，避免和业务逻辑混在一起。
- 发现某个功能适合回馈上游时，从 `main` 单独拉一个 PR 分支，不要直接拿 `custom` 提 PR。

## 每次上游发布后的检查清单

- [ ] `git fetch upstream --tags` 已执行。
- [ ] `main` 已通过 `--ff-only` 更新到 `upstream/main` 或指定 release tag。
- [ ] 没有把自定义功能提交到 `main`。
- [ ] 已在 `custom` 直接完成 `main -> custom` 合并；如使用临时验证分支，已合回 `custom` 并计划删除。
- [ ] 所有冲突已解决，没有遗漏 `<<<<<<<`、`=======`、`>>>>>>>`。
- [ ] `make test-backend` 通过。
- [ ] `make test-frontend` 通过。
- [ ] `make build` 通过。
- [ ] `custom` 已推送到 `origin`。

## 每次正式发布检查清单

- [ ] 当前分支是 `custom`。
- [ ] 工作区干净。
- [ ] `custom` 已包含本次要发布的上游 `main` 和功能提交。
- [ ] `backend/cmd/server/VERSION` 已更新为 `x.y.z-zhdgzs.n`。
- [ ] `docs/DOCKER_RELEASE_HISTORY.md` 已更新，且功能记录只统计自定义提交。
- [ ] 版本号 commit 已推送到 `origin/custom`。
- [ ] 已创建 annotated tag：`release/vx.y.z-zhdgzs.n`。
- [ ] release tag 已推送到 `origin`。
- [ ] Docker 镜像从同一个 `custom` commit 构建。
- [ ] Docker 镜像包含版本 tag 和 `custom` tag。
