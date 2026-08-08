#!/usr/bin/env python3
"""Synchronize Wei-Shaw/sub2api into this fork and preview conflicts."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path


UPSTREAM_NAME = "upstream"
ORIGIN_NAME = "origin"
MAIN_BRANCH = "main"
UPSTREAM_URL = "https://github.com/Wei-Shaw/sub2api.git"


@dataclass(frozen=True)
class CommandResult:
    stdout: str
    stderr: str
    returncode: int


class SyncError(RuntimeError):
    pass


def run_git(repo: Path, args: list[str], check: bool = True) -> CommandResult:
    result = subprocess.run(
        ["git", *args],
        cwd=repo,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    command = "git " + " ".join(args)
    if check and result.returncode != 0:
        details = (result.stderr or result.stdout).strip()
        raise SyncError(f"{command} failed: {details}")
    return CommandResult(result.stdout, result.stderr, result.returncode)


def require_git_repo(repo: Path) -> Path:
    result = run_git(repo, ["rev-parse", "--show-toplevel"], check=False)
    if result.returncode != 0:
        raise SyncError(f"{repo} is not a Git repository")
    return Path(result.stdout.strip()).resolve()


def current_branch(repo: Path) -> str:
    result = run_git(repo, ["branch", "--show-current"])
    branch = result.stdout.strip()
    if not branch:
        raise SyncError("detached HEAD is not supported")
    return branch


def require_clean_tree(repo: Path) -> None:
    status = run_git(repo, ["status", "--porcelain=v1"]).stdout.strip()
    if status:
        raise SyncError(
            "working tree is not clean; commit, stash, or discard local changes before syncing"
        )


def ref_or_empty(repo: Path, ref: str) -> str:
    result = run_git(repo, ["rev-parse", "--verify", ref], check=False)
    return result.stdout.strip() if result.returncode == 0 else ""


def short_sha(repo: Path, ref: str) -> str:
    result = run_git(repo, ["rev-parse", "--short", ref], check=False)
    return result.stdout.strip() if result.returncode == 0 else "(missing)"


def remote_url(repo: Path, remote: str) -> str:
    result = run_git(repo, ["remote", "get-url", remote], check=False)
    return result.stdout.strip() if result.returncode == 0 else ""


def ensure_remotes(repo: Path, execute: bool) -> None:
    origin_url = remote_url(repo, ORIGIN_NAME)
    if not origin_url:
        raise SyncError("origin remote is missing")
    if "Wei-Shaw/sub2api" in origin_url:
        raise SyncError("origin points to upstream; refusing to push to Wei-Shaw/sub2api")

    upstream_url = remote_url(repo, UPSTREAM_NAME)
    if upstream_url and upstream_url != UPSTREAM_URL:
        raise SyncError(f"upstream remote is {upstream_url}, expected {UPSTREAM_URL}")

    if not execute:
        return

    if not upstream_url:
        run_git(repo, ["remote", "add", UPSTREAM_NAME, UPSTREAM_URL])
    run_git(repo, ["remote", "set-url", "--push", UPSTREAM_NAME, "DISABLED"])


def commit_count(repo: Path, rev_range: str) -> int:
    result = run_git(repo, ["rev-list", "--count", rev_range], check=False)
    if result.returncode != 0:
        return 0
    raw = result.stdout.strip()
    return int(raw) if raw.isdigit() else 0


def upstream_summary(repo: Path, before_ref: str, after_ref: str) -> str:
    if not before_ref:
        base = run_git(repo, ["rev-list", "--max-parents=0", after_ref]).stdout.splitlines()
        before_ref = base[0] if base else after_ref

    if before_ref == after_ref:
        return "上游 main 没有新增提交。"

    rev_range = f"{before_ref}..{after_ref}"
    count = commit_count(repo, rev_range)
    logs = run_git(
        repo,
        ["log", "--oneline", "--no-decorate", "-n", "8", rev_range],
        check=False,
    ).stdout.strip()
    stat = run_git(
        repo,
        ["diff", "--stat", "--compact-summary", before_ref, after_ref],
        check=False,
    ).stdout.strip()

    lines = [f"上游新增提交：{count} 个。"]
    if logs:
        lines.append("主要提交：")
        lines.extend(f"- {line}" for line in logs.splitlines())
    if stat:
        stat_lines = stat.splitlines()
        lines.append("文件变化摘要：")
        lines.extend(f"- {line}" for line in stat_lines[:12])
        if len(stat_lines) > 12:
            lines.append(f"- ... 另有 {len(stat_lines) - 12} 行 diffstat")
    return "\n".join(lines)


def print_confirmation(repo: Path, branch: str) -> None:
    print("预检完成，尚未执行 git push 或 git pull。")
    print(f"仓库：{repo}")
    print(f"当前分支：{branch}")
    print()
    print("⚠️ 危险操作检测！")
    print("操作类型：同步 Wei-Shaw/sub2api 到 origin/main，并用 git pull 更新本地 main")
    print("影响范围：会 fetch upstream/origin，推送 origin/main，更新本地 main；不会合并当前 custom/feature 分支")
    print("风险评估：如果远端配置错误或 main 偏离上游，可能导致推送失败；未提交工作区会中止以避免覆盖改动")
    print()
    print('请确认是否继续？[需要明确的"是"、"确认"、"继续"]')
    print()
    print("确认后运行：")
    print('python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD" --yes')


def fetch_upstream(repo: Path) -> None:
    run_git(repo, ["fetch", UPSTREAM_NAME, "--tags"])


def fetch_origin_main(repo: Path) -> None:
    run_git(repo, ["fetch", ORIGIN_NAME, MAIN_BRANCH])


def sync_origin_main(repo: Path) -> None:
    source = f"refs/remotes/{UPSTREAM_NAME}/{MAIN_BRANCH}"
    target = f"refs/heads/{MAIN_BRANCH}"
    run_git(repo, ["push", ORIGIN_NAME, f"{source}:{target}"])
    run_git(repo, ["fetch", ORIGIN_NAME, MAIN_BRANCH])


def local_main_exists(repo: Path) -> bool:
    return bool(ref_or_empty(repo, f"refs/heads/{MAIN_BRANCH}"))


def ensure_local_main(repo: Path) -> None:
    if local_main_exists(repo):
        return
    run_git(repo, ["branch", "--track", MAIN_BRANCH, f"{ORIGIN_NAME}/{MAIN_BRANCH}"])


def pull_main(repo: Path, original_branch: str) -> None:
    ensure_local_main(repo)
    if original_branch == MAIN_BRANCH:
        run_git(repo, ["pull", "--ff-only", ORIGIN_NAME, MAIN_BRANCH])
        return

    tmp_parent = Path(tempfile.mkdtemp(prefix="sub2api-main-pull-"))
    worktree_path = tmp_parent / "main"
    try:
        run_git(repo, ["worktree", "add", "--quiet", str(worktree_path), MAIN_BRANCH])
        run_git(worktree_path, ["pull", "--ff-only", ORIGIN_NAME, MAIN_BRANCH])
    finally:
        if worktree_path.exists():
            run_git(repo, ["worktree", "remove", "--force", str(worktree_path)], check=False)
        run_git(repo, ["worktree", "prune"], check=False)
        shutil.rmtree(tmp_parent, ignore_errors=True)


def conflict_preview(repo: Path, branch: str) -> tuple[bool, str]:
    if branch == MAIN_BRANCH:
        return False, "当前分支是 main，跳过冲突预检。"

    result = run_git(
        repo,
        [
            "merge-tree",
            "--write-tree",
            "--messages",
            "--name-only",
            branch,
            MAIN_BRANCH,
        ],
        check=False,
    )
    output = (result.stdout + result.stderr).strip()
    if result.returncode == 0:
        return False, f"冲突预检：将更新后的 main 合入 {branch} 未发现冲突。"
    if result.returncode == 1:
        detail = output or "git merge-tree reported conflicts without path details"
        return True, f"冲突预检：将更新后的 main 合入 {branch} 可能产生冲突。\n{detail}"
    raise SyncError(f"git merge-tree failed: {output}")


def run_sync(repo: Path) -> None:
    original_branch = current_branch(repo)
    require_clean_tree(repo)
    ensure_remotes(repo, execute=True)

    fetch_origin_main(repo)
    before_origin_main = ref_or_empty(repo, f"refs/remotes/{ORIGIN_NAME}/{MAIN_BRANCH}")
    before_local_main = ref_or_empty(repo, f"refs/heads/{MAIN_BRANCH}")

    fetch_upstream(repo)
    upstream_main = ref_or_empty(repo, f"refs/remotes/{UPSTREAM_NAME}/{MAIN_BRANCH}")
    if not upstream_main:
        raise SyncError("upstream/main was not fetched")

    summary_base = before_origin_main or before_local_main
    summary = upstream_summary(repo, summary_base, upstream_main)

    sync_origin_main(repo)
    pull_main(repo, original_branch)
    has_conflict, conflict_message = conflict_preview(repo, original_branch)

    after_origin_main = short_sha(repo, f"refs/remotes/{ORIGIN_NAME}/{MAIN_BRANCH}")
    after_local_main = short_sha(repo, f"refs/heads/{MAIN_BRANCH}")
    before_origin_short = before_origin_main[:12] if before_origin_main else "(missing)"
    before_local_short = before_local_main[:12] if before_local_main else "(missing)"

    print("同步完成。")
    print(f"origin/main：{before_origin_short} -> {after_origin_main}")
    print(f"local main：{before_local_short} -> {after_local_main}")
    print()
    print(summary)
    print()
    print(conflict_message)
    if has_conflict:
        sys.exit(2)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Sync Wei-Shaw/sub2api upstream into origin/main, pull local main, and preview conflicts."
    )
    parser.add_argument("--repo", default=os.getcwd(), help="repository path")
    parser.add_argument(
        "--yes",
        action="store_true",
        help="perform git fetch, push, pull, and conflict preview",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        repo = require_git_repo(Path(args.repo).resolve())
        branch = current_branch(repo)
        require_clean_tree(repo)
        ensure_remotes(repo, execute=args.yes)
        if not args.yes:
            print_confirmation(repo, branch)
            return 0
        run_sync(repo)
        return 0
    except SyncError as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
