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

Before the first mutating execution, show this single combined confirmation unless the user already gave clear approval in the current turn. This one confirmation covers the embedded `sync-upstream` step and the remote Docker release. Do not ask for a separate `sync-upstream` confirmation, and do not ask for a second version confirmation when using the script's candidate version.

```text
⚠️ 危险操作检测！
操作类型：remote Docker 发布流程
影响范围：会同步 upstream/main 到 origin/main 和本地 main，切换到 custom，合并 main 和调用时所在开发分支，更新 backend/cmd/server/VERSION 和 docs/DOCKER_RELEASE_HISTORY.md，提交 release 元数据，推送 origin/custom，触发 GitHub Actions 构建/推送正式 Docker 镜像，并在本机后台启动 /root/sub2api-deploy/watch_remote_docker_deploy.sh
风险评估：如果分支选择或版本号错误，会影响正式发布追溯；发布预检会列出 `main → custom` 与 `source → custom` 的全部冲突及合并建议，存在语义冲突时流程会在修改 custom 前停止，等待一次性确认；不会自动选边、自动解决冲突、自动 stash 或提交用户未提交改动；GitHub Actions 负责构建和推送镜像，部署 watcher 会在后台轮询构建结果并自行拉取镜像、执行 docker compose up -d 和发送飞书通知，agent 不等待该脚本完成

请确认是否继续？[需要明确的"是"、"确认"、"继续"]
```

Do not auto-stash, auto-resolve conflicts, delete branches, delete tags, force-push, push to `upstream`, wait for the GitHub Actions build to finish, or call production deploy/restart endpoints manually.

Before every release merge, run the read-only risk assessment. Report all text conflicts and all high-risk shared edits that Git would auto-merge from both `main → custom` and `source → custom` in one response. For each path, state the recommended strategy and the required verification. When any recommendation requires a product or code-semantic choice, stop and request one consolidated user decision before modifying `custom`. In i18n files, explicitly check that object keys remain unique; a clean Git merge does not guarantee valid TypeScript.

Treat generated files as derived artifacts: resolve their source definitions first, regenerate when tooling is available, and never resolve a generated-file conflict by blindly choosing one side.

Use the candidate version from the post-sync preview by default. If the user supplied an explicit version in the initial request, use it only if it validates against the synced `main` base version. Do not use `--allow-version-override` unless that override was explicitly approved before the first mutating command; otherwise stop and report the validation error instead of starting a second confirmation round.

## Required Files

Remote builds require:

```text
.github/workflows/remote-docker.yml
.agents/skills/sync-upstream/scripts/sync_upstream.py
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
5. If the user has not already given clear approval in the current turn, show the single combined high-risk confirmation from **Safety Rules**. After that confirmation, continue without any second confirmation.
6. Run `sync-upstream` first so `main` is current before choosing the release version:

   ```bash
   python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD" --yes
   ```

7. Run the remote release script in preview mode after the sync. Review its `merge_risk_assessment`, which reports conflicts on the actual release merge paths:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD"
   ```

8. Review the proposed source branch, `main`, `custom`, version candidate, GHCR tags, workflow file, release record file, and every conflict recommendation.
9. If conflicts exist, present the complete list with recommendations in one response and wait for a single user decision. Resolve the approved decisions, run applicable quality checks, and repeat the preview until no unresolved conflicts remain.
10. Choose the script's `candidate_version` unless the user already supplied an explicit valid version in the initial request.
11. Execute immediately after the conflict-free preview with the chosen version:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD" --yes --version <chosen-version>
   ```

12. Report the release commit, GHCR image tags, workflow URL, deploy watcher PID, and deploy watcher log path.
13. Treat the task as complete after the workflow is triggered and the deploy watcher script is started. Do not wait for the watcher, poll Actions from the agent, or manually restart production.

## Script Contract

`scripts/remote_docker.py` is the deterministic sequence for this skill.

Preview mode must not mutate the repository.

Execution mode:

- Requires `--yes` and `--version`.
- Requires the initial source branch worktree to be clean.
- Calls the project `sync-upstream` script with `--yes`. A conflict-preview exit code is retained so the script can report all release-path conflicts.
- Checks out `custom`.
- Pulls `origin/custom` with `--ff-only`.
- Merges `main` into `custom` with `--no-ff` when needed.
- Merges the recorded source branch into `custom` with `--no-ff` unless already contained.
- Performs a read-only risk assessment before checkout or merge. If any text conflict or high-risk auto-merged shared edit exists, prints every affected path with its recommendation and exits without modifying `custom`.
- Does not auto-resolve conflicts. After reviewed decisions are applied and the preview is conflict-free, normal merges create their own commits.
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
