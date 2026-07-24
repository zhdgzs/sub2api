---
name: release-docker
description: "Prepare and build a Sub2API release Docker image from the custom branch. Use when the user asks to package a release Docker image, publish/build the custom Docker release, update the fork release version, create a release/* tag, or merge the current development branch plus latest upstream main into custom before Docker build. This skill is release-only, not for local/dev image builds."
---

# Release Docker

## Scope

Use this skill only for release-mode Docker builds for this fork. It enforces the repository model in `docs/SYNC_UPSTREAM_CN.md`:

- `main` is only an upstream mirror.
- `custom` is the only formal Docker release build branch.
- The caller's current branch is treated as the source development branch.
- `release/*` is an annotated tag namespace, not a regular branch.
- `sync/*` is not a regular release source.

Do not use this skill for quick local images such as `sub2api:local`.

## Safety Rules

This workflow performs high-risk operations: branch checkout, pull, merge, commit, push, tag creation, tag push, and Docker build.

Before any mutating execution, show this confirmation:

```text
⚠️ 危险操作检测！
操作类型：release Docker 打包流程
影响范围：会同步 upstream/main 到 origin/main 和本地 main，切换到 custom，合并 main 和调用时所在开发分支，更新 backend/cmd/server/VERSION 和 docs/DOCKER_RELEASE_HISTORY.md，提交 release 元数据，推送 origin/custom，创建并推送 release/* tag，构建正式 Docker 镜像
风险评估：如果分支选择、版本号或 tag 错误，会影响正式发布追溯；如果合并冲突，流程会中止并要求人工解决；不会自动解决冲突，不会自动 stash 或提交未提交改动

请确认是否继续？[需要明确的"是"、"确认"、"继续"]
```

Do not auto-stash, auto-resolve conflicts, delete branches, delete tags, force-push, or push to `upstream`.

## Docker Cache Rules

Release Docker builds must preserve the local Docker build cache by default.

- Do not run `docker system prune`, `docker builder prune`, `docker image prune`, `docker rmi`, or equivalent cache/image cleanup as part of this workflow.
- Do not add `--no-cache` to release builds unless the user explicitly asks for a no-cache rebuild.
- Do not create and remove a temporary BuildKit builder in a way that discards cache between release builds.
- If the user asks to clean cache, prune images, or force a no-cache build, treat it as a separate high-risk operation and require explicit confirmation before doing it.
- When diagnosing a slow release build, first inspect cache hits with `DOCKER_BUILDKIT=1 docker build --progress=plain ...` or equivalent output instead of clearing cache.

## Release Validation Scope

`$release-docker` is a release packaging workflow, not a test or QA workflow.
By default, it must perform only the minimum checks required to package and
trace a release plus the production Docker build.

- Do not run unit, integration, E2E, or frontend test commands as part of this
  skill. This includes `go test`, `go test -run`, `go test ./...`, `pnpm test`,
  `pnpm test:run`, `vitest`, `playwright`, `make test-*`, and equivalent
  commands.
- Do not run test compilation or standalone validation builds as part of this
  skill. This includes `go test -c`, `go test` used only to compile test
  packages, standalone `pnpm typecheck`, `vue-tsc`, `tsc --noEmit`, and
  equivalent commands.
- Do not run an extra project build outside the release script as a default
  quality gate. The release Docker build is the only default build validation.
- It is allowed for the Dockerfile itself to run the production build steps
  required to produce the image, such as frontend production build and backend
  `go build`; these are not extra test gates.
- If the user explicitly asks for tests, type-checks, or extra validation in the
  same turn, run them as a separate pre-release validation step and report that
  they were user-requested. Do not silently add them to `$release-docker`.

## Standard Workflow

1. Read `docs/SYNC_UPSTREAM_CN.md`.
2. Confirm the current directory is the intended `sub2api` repository.
3. Record the current branch as the source branch.
4. Require a clean working tree before starting.
5. Run `$sync-upstream` or its script to update `main`.
6. Run the release script in preview mode:

   ```bash
   python3 ".agents/skills/release-docker/scripts/release_docker.py" --repo "$PWD"
   ```

7. Review the proposed source branch, `main`, `custom`, version candidate, release tag, Docker tags, and release record file.
8. Ask the user to confirm the version.
9. Execute after explicit confirmation:

   ```bash
   python3 ".agents/skills/release-docker/scripts/release_docker.py" --repo "$PWD" --yes --version <confirmed-version>
   ```

10. After the build completes, verify release metadata with lightweight Git/file
    checks only; do not run tests or standalone type-checks.
11. Report the commit, tag, Docker tags, release record, and any warnings.

## Script Contract

`scripts/release_docker.py` is the source of truth for the deterministic sequence.

Preview mode must not mutate the repository. It checks state and prints the execution plan.

Execution mode:

- Requires `--yes` and `--version`.
- Requires the initial source branch worktree to be clean.
- Calls the project `sync-upstream` script with `--yes`.
- Checks out `custom`.
- Pulls `origin/custom` with `--ff-only`.
- Merges `main` into `custom` with `--no-ff`.
- Merges the recorded source branch into `custom` with `--no-ff` unless already contained.
- Stops on merge conflicts.
- Writes `backend/cmd/server/VERSION`.
- Writes/updates `docs/DOCKER_RELEASE_HISTORY.md`.
- Generates the release record from this fork's custom commits only, excluding `main` / `upstream` commits.
- Commits `chore(release): <version>` only if release metadata changed.
- Pushes `origin/custom`.
- Creates annotated tag `release/v<version>`.
- Pushes the release tag to `origin`.
- Builds Docker tags:
  - `sub2api:<version>`
  - `sub2api:custom`

The script does not run test suites or standalone test compilation. Docker build
is the only default build validation. If the user explicitly asks for extra
validation, run it before execution or before Docker build as a separate,
user-requested step.

## Version Rules

The candidate version is based on `main:backend/cmd/server/VERSION` and existing `release/v<base>-zhdgzs.<n>` tags:

- If no release tag exists for the base version, use `<base>-zhdgzs.1`.
- If release tags exist, increment the highest numeric suffix.
- If the user supplies `--version`, validate it matches `<base>-zhdgzs.<n>` unless the user explicitly confirms an override.

Docker tags do not use `release/` because Docker tags cannot contain `/`.

## Release Record Rules

Formal release build history lives in:

```text
docs/DOCKER_RELEASE_HISTORY.md
```

Every successful release Docker build must leave exactly one record row for the built version:

- One sentence describing the functionality included in this build.
- The sentence must be based on this fork's custom commits only.
- Do not count upstream/main commits in the functional summary.
- If the version already has a row, update that row instead of appending a duplicate.
