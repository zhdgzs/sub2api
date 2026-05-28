# Git 操作规范与 Fork 维护流程

本仓库是 `Wei-Shaw/sub2api` 的 fork。长期维护目标是：`main` 尽量保持上游原样，所有自定义功能只进入 `custom`，单个功能在 `feature/<name>` 中开发。

本文件集中维护本仓库的 Git 操作约束、分支模型、上游同步流程、发布流程和长期定制维护规则。任何 agent 在执行或建议 `git commit`、`git push`、`git pull`，或处理上游同步、分支合并、发布、长期定制维护前，必须先阅读本文件。

## Agent Git 操作约束

执行 `git commit` 前：

- 先确认当前分支，避免把自定义改动提交到 `main`。
- 如果当前分支是 `main`，只允许上游同步类提交；其他改动应切到 `custom` 或 `feature/<name>`。
- 检查 `git status --short --branch`，确认没有混入无关改动。
- 涉及数据库 schema 或迁移的改动应单独提交。

执行 `git pull` 前：

- 先确认当前分支和远端跟踪关系。
- `main` 只能从 `origin/main` 或 `upstream/main` 更新。
- `custom` 只能从 `origin/custom` 更新。
- 不要在有未提交改动时直接 pull，除非用户明确要求并已说明冲突风险。

执行 `git push` 前：

- 先确认推送目标是 `origin`。
- 不要推送到 `upstream`。
- `main` 只推送上游镜像更新。
- 自定义功能只能推送到 `custom`、`feature/*`、`sync/*` 或 `release/*`。

通用变更卫生：

- 不要回滚用户未要求回滚的改动。
- 不要使用破坏性 Git 命令，例如 `git reset --hard` 或 `git checkout -- <file>`，除非用户明确要求。
- 新增前端依赖时，必须同步提交 `frontend/pnpm-lock.yaml`。
- 适合回馈上游的功能应从 `main` 单独拉 PR 分支，不要直接拿 `custom` 提上游 PR。

## 分支职责

严格按下面的职责使用分支：

- `main`: 上游镜像分支，永远尽量接近 `upstream/main`，不提交自己的功能、配置、文档或部署改动。
- `custom`: 长期定制分支，用于部署自己的版本，所有自定义功能最终合并到这里。
- `feature/<name>`: 单个功能开发分支，从 `custom` 拉出，完成后合并回 `custom`。

可选临时分支：

- `sync/custom-v<version>`: 上游大版本升级时，用来预演 `main` 合并进 `custom`，测试通过后再合并回 `custom`。
- `release/<version>`: 如需单独打包或灰度测试自己的发布版本，可临时创建。

核心规则：`main` 只负责接收上游，`custom` 才是自己的产品线。

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
git pull origin custom

git checkout -b feature/my-feature
# 开发并提交
git push origin feature/my-feature
```

功能完成后合并回 `custom`，不要合并到 `main`：

```bash
git checkout custom
git pull origin custom
git merge --no-ff feature/my-feature
git push origin custom
```

## 同步上游新版

上游发布新版后，先更新 `main`，再把 `main` 合并进 `custom`。

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
git pull origin custom

git checkout -b sync/custom-v0.1.133
git merge --no-ff main
```

如果发生冲突：

```bash
git status
# 手工解决冲突文件
git add <resolved-files>
git commit
```

合并完成后执行验证：

```bash
make test-backend
make test-frontend
make build
```

验证通过后合并回 `custom`：

```bash
git checkout custom
git merge --no-ff sync/custom-v0.1.133
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

## 冲突控制原则

长期维护 fork 的关键是减少 `custom` 和上游核心代码的重叠修改：

- 优先新增配置项、独立 service、独立 handler、独立前端组件。
- 避免大面积重写上游调度、鉴权、计费、迁移等核心路径。
- 必须改核心逻辑时，把改动拆成小 commit，并写清楚原因。
- 新增依赖时，前端必须同步提交 `frontend/pnpm-lock.yaml`。
- 数据库 schema 或迁移相关改动要单独提交，避免和业务逻辑混在一起。
- 发现某个功能适合回馈上游时，从 `main` 单独拉一个 PR 分支，不要直接拿 `custom` 提 PR。

## 自定义版本命名

建议使用上游版本号加自己的后缀：

```text
0.1.132-zhdgzs.1
0.1.133-zhdgzs.1
0.1.133-zhdgzs.2
```

含义：

- `0.1.133`: 对应上游基线版本。
- `zhdgzs.1`: 自己 fork 的第 1 个定制发布。

这样部署、回滚和排查问题时，可以快速区分是上游问题还是 fork 自定义问题。

## 每次上游发布后的检查清单

- [ ] `git fetch upstream --tags` 已执行。
- [ ] `main` 已通过 `--ff-only` 更新到 `upstream/main` 或指定 release tag。
- [ ] 没有把自定义功能提交到 `main`。
- [ ] 在 `sync/custom-v<version>` 分支完成 `main -> custom` 合并。
- [ ] 所有冲突已解决，没有遗漏 `<<<<<<<`、`=======`、`>>>>>>>`。
- [ ] `make test-backend` 通过。
- [ ] `make test-frontend` 通过。
- [ ] `make build` 通过。
- [ ] 合并回 `custom` 并推送到 `origin`。
