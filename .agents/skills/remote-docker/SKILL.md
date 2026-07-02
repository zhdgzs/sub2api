---
name: remote-docker
description: "Prepare a Sub2API custom release from the current development branch, push origin/custom and a release/vX tag, then let GitHub Actions build and push the official Docker image. Use when local release-docker builds are too slow and the release image should be built remotely by GitHub instead of local Docker."
---

# Remote Docker

## Scope

Use this skill only for release-mode Docker builds for this fork when the Docker build must run on GitHub Actions instead of the local machine. It follows the repository model in `docs/SYNC_UPSTREAM_CN.md`:

- `main` is only an upstream mirror.
- `custom` is the only formal Docker release build branch.
- The caller's current branch is treated as the source development branch.
- `release/*` is an annotated tag namespace, not a regular branch.
- GitHub Actions builds from the pushed `release/v<version>` tag.

Do not modify or invoke `release-docker` execution mode for remote builds. This skill may reuse the existing release helper functions, but its script is the source of truth for the remote-build sequence.

## Safety Rules

This workflow performs high-risk operations: branch checkout, pull, merge, commit, push, tag creation, tag push, and a remote Docker release build.

Before any mutating execution, show this confirmation:

```text
⚠️ 危险操作检测！
操作类型：remote Docker 发布流程
影响范围：会同步 upstream/main 到 origin/main 和本地 main，切换到 custom，合并 main 和调用时所在开发分支，更新 backend/cmd/server/VERSION 和 docs/DOCKER_RELEASE_HISTORY.md，提交 release 元数据，推送 origin/custom，创建并推送 release/* tag，并触发 GitHub Actions 构建/推送正式 Docker 镜像
风险评估：如果分支选择、版本号或 tag 错误，会影响正式发布追溯；如果合并冲突，流程会中止并要求人工解决；不会自动解决冲突，不会自动 stash 或提交未提交改动；如果部署 webhook 已配置，镜像构建成功后可能触发远端服务重启

请确认是否继续？[需要明确的"是"、"确认"、"继续"]
```

Do not auto-stash, auto-resolve conflicts, delete branches, delete tags, force-push, push to `upstream`, or call production deploy/restart endpoints manually.

## Required Repository Files

Remote builds require:

```text
.github/workflows/remote-docker.yml
.agents/skills/remote-docker/scripts/remote_docker.py
```

The workflow builds the root `Dockerfile` from the pushed `release/v<version>` tag and pushes:

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

6. Review the proposed source branch, `main`, `custom`, version candidate, release tag, GHCR tags, workflow file, and release record file.
7. Ask the user to confirm the version and the high-risk operation.
8. Execute after explicit confirmation:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD" --yes --version <confirmed-version>
   ```

9. Report the release commit, tag, GHCR image tags, workflow URL, and deploy webhook status.
10. Do not wait for or restart production unless the user explicitly asks and the endpoint is already configured.

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
- Creates annotated tag `release/v<version>`.
- Pushes the release tag to `origin`.
- Does not run local Docker build.
- Relies on `.github/workflows/remote-docker.yml` to build and push GHCR images from the tag.

## Hook and Restart

GitHub supports repository webhooks and workflow events, but this skill uses a simpler opt-in deploy webhook from the Actions workflow:

- Configure repository secret `REMOTE_DOCKER_WEBHOOK_URL` to receive a POST after a successful image push.
- Optionally configure `REMOTE_DOCKER_WEBHOOK_TOKEN`; the workflow sends it as `Authorization: Bearer <token>`.
- The receiving server should verify the token, pull `ghcr.io/<owner>/sub2api:<version>`, and restart with its own deployment script.
- Do not put restart commands or server credentials directly in the workflow.
- If no webhook URL secret is configured, the workflow only builds and pushes the image.

Manual `workflow_dispatch` is present for reruns, but tag push is the primary trigger because this fork keeps `main` as an upstream mirror.
