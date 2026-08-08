---
name: sync-upstream
description: "同步 Wei-Shaw/sub2api 上游 main 到当前 fork 的 origin/main，随后更新本地 main；不合并 cust、不创建 sync/*，当调用时所在分支不是 main 时只做 main 合入当前分支的冲突预检并输出简短更新摘要。用于用户要求同步上游、更新 fork、pull main，或检查当前分支是否会与新 main 冲突的场景。"
---

# Sync Upstream

## Overview

Use this skill to keep this fork's `main` aligned with `Wei-Shaw/sub2api` while preserving the project rule that `main` is only an upstream mirror and cust work stays outside `main`.

This skill only performs the upstream mirror sync:

- `upstream/main` -> `origin/main`
- `origin/main` -> local `main`

It does not merge `main` into `cust`, does not create `sync/*` branches, and does not prepare releases. Merging `main` into `cust` is a separate workflow.

## Safety Gate

Before running any mutating command, read `docs/SYNC_UPSTREAM_CN.md` from the repository root and follow it. This workflow performs `git push` and `git pull`, so require explicit confirmation unless the user already gave a clear approval in the current turn.

Use this confirmation format:

```text
⚠️ 危险操作检测！
操作类型：同步 Wei-Shaw/sub2api 到 origin/main，并用 git pull 更新本地 main
影响范围：会 fetch upstream/origin，推送 origin/main，更新本地 main；不会合并 cust，不会创建 sync/*，不会修改当前非 main 分支
风险评估：如果远端配置错误或 main 偏离上游，可能导致推送失败；未提交工作区会中止以避免覆盖改动

请确认是否继续？[需要明确的"是"、"确认"、"继续"]
```

Do not run `git commit`, `git reset --hard`, force-push, create/delete branches, create/delete tags, or merge `main` into `cust` as part of this skill.

## Workflow

1. Confirm the current directory is the intended `sub2api` repository.
2. Run the script without `--yes` to show local preflight status and the required confirmation block:

   ```bash
   python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD"
   ```

3. After explicit confirmation, execute:

   ```bash
   python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD" --yes
   ```

4. Report the script result in Chinese, including:
   - whether `origin/main` and local `main` changed;
   - the short upstream update summary;
   - if the original branch was not `main`, whether a virtual merge of updated `main` into that branch would conflict;
   - remind that updating `cust` is a separate explicit step if the user asks about Docker/release readiness.

## Script Contract

`scripts/sync_upstream.py` is the source of truth for the exact Git sequence.

- It aborts on dirty working trees before mutating anything.
- It ensures `upstream` points to `https://github.com/Wei-Shaw/sub2api.git` and disables upstream push.
- It fetches `upstream` and `origin`.
- It pushes `refs/remotes/upstream/main` to `origin/main` without force.
- It updates local `main` by running `git pull --ff-only origin main`; from non-`main` branches it uses a temporary worktree so the caller's branch is not checked out or merged.
- It checks conflict risk with `git merge-tree --write-tree` only; it does not modify the current branch.
- It prints a concise commit and diffstat summary for the upstream range that was newly synced.

If the script stops with a non-zero exit code, report the blocking reason and do not invent a manual recovery unless the user asks.
