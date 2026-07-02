---
name: remote-docker
description: "Prepare a Sub2API custom release from the current development branch, push origin/custom, then let GitHub Actions build and push the official Docker image from the custom branch. Use when local release-docker builds are too slow and the release image should be built remotely by GitHub instead of local Docker."
---

# Remote Docker

## Scope

Use this skill only for release-mode Docker builds for this fork when the Docker build must run on GitHub Actions instead of the local machine. It follows the repository model in `docs/SYNC_UPSTREAM_CN.md`:

- `main` is only an upstream mirror.
- `custom` is the only formal Docker release build branch.
- The caller's current branch is treated as the source development branch.
- GitHub Actions builds from the pushed `custom` branch.

Do not modify or invoke `release-docker` execution mode for remote builds. This skill may reuse the existing release helper functions, but its script is the source of truth for the remote-build sequence.

## Safety Rules

This workflow performs high-risk operations: branch checkout, pull, merge, commit, push, and a remote Docker release build.

Before any mutating execution, show this confirmation:

```text
⚠️ 危险操作检测！
操作类型：remote Docker 发布流程
影响范围：会同步 upstream/main 到 origin/main 和本地 main，切换到 custom，合并 main 和调用时所在开发分支，更新 backend/cmd/server/VERSION 和 docs/DOCKER_RELEASE_HISTORY.md，提交 release 元数据，推送 origin/custom，触发 GitHub Actions 构建/推送正式 Docker 镜像，并在本机后台启动 /root/sub2api-deploy/watch_remote_docker_deploy.sh
风险评估：如果分支选择或版本号错误，会影响正式发布追溯；如果合并冲突，流程会中止并要求人工解决；不会自动解决冲突，不会自动 stash 或提交未提交改动；GitHub Actions 负责构建和推送镜像，部署 watcher 会在后台轮询构建结果并自行拉取镜像、执行 docker compose up -d 和发送飞书通知，agent 不等待该脚本完成

请确认是否继续？[需要明确的"是"、"确认"、"继续"]
```

Do not auto-stash, auto-resolve conflicts, delete branches, delete tags, force-push, push to `upstream`, wait for the GitHub Actions build to finish, or call production deploy/restart endpoints manually.

## Required Files

Remote builds require:

```text
.github/workflows/remote-docker.yml
.agents/skills/remote-docker/scripts/remote_docker.py
/root/sub2api-deploy/watch_remote_docker_deploy.sh
```

The workflow builds the root `Dockerfile` from the pushed `custom` branch and pushes:

```text
ghcr.io/<owner>/sub2api:<version>
ghcr.io/<owner>/sub2api:custom
```

## Standard Workflow

1. Read `docs/SYNC_UPSTREAM_CN.md`.
2. Confirm the current directory is the intended `sub2api` repository.
3. Record the current branch as the source branch.
4. Require a clean working tree before starting.
5. Run the remote release script in preview mode:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD"
   ```

6. Review the proposed source branch, `main`, `custom`, version candidate, GHCR tags, workflow file, and release record file.
7. Ask the user to confirm the version and the high-risk operation.
8. Execute after explicit confirmation:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD" --yes --version <confirmed-version>
   ```

9. Report the release commit, GHCR image tags, workflow URL, deploy watcher PID, and deploy watcher log path.
10. Treat the task as complete after the workflow is triggered and the deploy watcher script is started. Do not wait for the watcher, poll Actions from the agent, or manually restart production.

## Script Contract

`scripts/remote_docker.py` is the deterministic sequence for this skill.

Preview mode must not mutate the repository.

Execution mode:

- Requires `--yes` and `--version`.
- Requires the initial source branch worktree to be clean.
- Calls the project `sync-upstream` script with `--yes`.
- Checks out `custom`.
- Pulls `origin/custom` with `--ff-only`.
- Merges `main` into `custom` with `--no-ff` when needed.
- Merges the recorded source branch into `custom` with `--no-ff` unless already contained.
- Stops on merge conflicts.
- Writes `backend/cmd/server/VERSION`.
- Writes/updates `docs/DOCKER_RELEASE_HISTORY.md`.
- Commits `chore(release): <version>` only if release metadata changed.
- Pushes `origin/custom`.
- Starts `/root/sub2api-deploy/watch_remote_docker_deploy.sh` in the background after the push, with `EXPECTED_HEAD_SHA` set to the pushed `custom` HEAD.
- Does not run local Docker build.
- Relies on `.github/workflows/remote-docker.yml` to build and push GHCR images from `custom`.
- Does not wait for GitHub Actions or the deploy watcher to finish.

## Restart Policy

Remote Docker only triggers the remote build and starts the existing external deploy watcher. It must not configure deploy hooks, call restart endpoints, or assume any production deployment topology beyond invoking `/root/sub2api-deploy/watch_remote_docker_deploy.sh`.

The deploy watcher is responsible for polling GitHub Actions, pulling `ghcr.io/zhdgzs/sub2api:custom`, tagging `sub2api:custom`, running `docker compose up -d` in `/root/sub2api-deploy`, and sending Feishu notifications. The agent must not keep monitoring the build after this script is launched.

Manual `workflow_dispatch` is present for reruns, but `custom` branch push is the primary trigger because this fork keeps `main` as an upstream mirror.
