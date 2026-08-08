#!/usr/bin/env python3
"""Update docs/DOCKER_RELEASE_HISTORY.md from cust release commits."""

from __future__ import annotations

import argparse
import re
import subprocess
from dataclasses import dataclass
from datetime import date
from pathlib import Path


CONVENTIONAL_SUBJECT_RE = re.compile(r"^(?P<type>[a-z]+)(?:\([^)]+\))?!?:\s*")
RELEASE_COMMIT_SUBJECT_RE = re.compile(
    r"^chore\(release\):\s+(?:发布\s+)?[0-9]+\.[0-9]+\.[0-9]+-zhdgzs\.[0-9]+$"
)
RELEASE_COMMIT_GREP_RE = (
    r"^chore\(release\):[[:space:]]+(发布[[:space:]]+)?"
    r"[0-9]+\.[0-9]+\.[0-9]+-zhdgzs\.[0-9]+$"
)
RELEASE_RECORD_RELATIVE = Path("docs/DOCKER_RELEASE_HISTORY.md")
RELEASE_RECORD_HEADER = """# Docker 打包记录

> 仅记录 `cust` 分支的正式 release Docker 镜像。
> 每次通过 `release-docker` 或 `remote-docker` 完成发布时更新。
> 功能记录只统计 `cust` 相对 `main` 的自定义提交，不统计上游提交。

| 版本 | 打包日期 | 功能记录 | 来源提交 |
| --- | --- | --- | --- |
"""


@dataclass(frozen=True)
class ReleaseChange:
    commit: str
    subject: str


def run(args: list[str], *, cwd: Path, check: bool = True) -> subprocess.CompletedProcess[str]:
    proc = subprocess.run(
        args,
        cwd=cwd,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and proc.returncode != 0:
        cmd = " ".join(args)
        detail = (proc.stderr or proc.stdout or f"exit code {proc.returncode}").strip()
        raise SystemExit(f"[ERROR] {cmd}\n{detail}")
    return proc


def git(repo: Path, *args: str, check: bool = True) -> str:
    return run(["git", *args], cwd=repo, check=check).stdout.strip()


def require_repo(repo: Path) -> None:
    if not (repo / ".git").exists():
        raise SystemExit(f"[ERROR] Not a git repository: {repo}")
    if not (repo / "docs" / "SYNC_UPSTREAM_CN.md").exists():
        raise SystemExit("[ERROR] docs/SYNC_UPSTREAM_CN.md not found")


def exact_release_tag(repo: Path, rev: str) -> str | None:
    output = git(repo, "tag", "--points-at", rev, "--list", "release/v*")
    tags = [line.strip() for line in output.splitlines() if line.strip()]
    return tags[0] if tags else None


def nearest_release_tag(repo: Path, rev: str) -> str | None:
    proc = run(
        ["git", "describe", "--tags", "--abbrev=0", "--match", "release/v*", rev],
        cwd=repo,
        check=False,
    )
    if proc.returncode != 0:
        return None
    return proc.stdout.strip() or None


def is_release_commit(repo: Path, rev: str) -> bool:
    proc = run(["git", "log", "-1", "--format=%s", rev], cwd=repo, check=False)
    if proc.returncode != 0:
        return False
    return RELEASE_COMMIT_SUBJECT_RE.match(proc.stdout.strip()) is not None


def nearest_release_commit(repo: Path, rev: str) -> str | None:
    proc = run(
        [
            "git",
            "log",
            "--format=%H",
            "--max-count=1",
            "--grep",
            RELEASE_COMMIT_GREP_RE,
            "--extended-regexp",
            rev,
        ],
        cwd=repo,
        check=False,
    )
    if proc.returncode != 0:
        return None
    return proc.stdout.strip() or None


def previous_release_ref(repo: Path, rev: str) -> str | None:
    exact = exact_release_tag(repo, rev)
    lookup_rev = f"{rev}^" if exact or is_release_commit(repo, rev) else rev
    commit = nearest_release_commit(repo, lookup_rev)
    if commit:
        return commit
    return nearest_release_tag(repo, lookup_rev)


def cust_release_changes(repo: Path, rev: str) -> list[ReleaseChange]:
    previous = previous_release_ref(repo, rev)
    range_expr = f"{previous}..{rev}" if previous else rev
    output = git(
        repo,
        "log",
        "--reverse",
        "--no-merges",
        "--format=%h%x09%s",
        range_expr,
        "--not",
        "main",
    )
    changes: list[ReleaseChange] = []
    for line in output.splitlines():
        if not line.strip():
            continue
        commit, _, subject = line.partition("\t")
        subject = subject.strip()
        if not subject or subject.startswith("chore(release):"):
            continue
        changes.append(ReleaseChange(commit=commit.strip(), subject=subject))
    return changes


def release_note_fragment(subject: str) -> str:
    fragment = CONVENTIONAL_SUBJECT_RE.sub("", subject, count=1).strip()
    return fragment.rstrip("。；;,.! ")


def release_note(changes: list[ReleaseChange]) -> str:
    fragments: list[str] = []
    for change in changes:
        fragment = release_note_fragment(change.subject)
        if fragment:
            fragments.append(fragment)
    if not fragments:
        return "本次打包未包含新的自定义功能变更。"
    if len(fragments) == 1:
        return f"{fragments[0]}。"
    return "本次打包包含：" + "；".join(fragments) + "。"


def escape_table_cell(value: str) -> str:
    return value.replace("|", "\\|").replace("\n", "<br>")


def release_record_row(version: str, release_date: str, note: str, changes: list[ReleaseChange]) -> str:
    if changes:
        sources = "<br>".join(
            f"`{change.commit}` {escape_table_cell(change.subject)}" for change in changes
        )
    else:
        sources = "无"
    return f"| `v{version}` | `{release_date}` | {escape_table_cell(note)} | {sources} |"


def update_release_record(repo: Path, version: str, release_date: str, rev: str) -> tuple[bool, str]:
    record_file = repo / RELEASE_RECORD_RELATIVE
    record_file.parent.mkdir(parents=True, exist_ok=True)
    changes = cust_release_changes(repo, rev)
    note = release_note(changes)
    row = release_record_row(version, release_date, note, changes)
    if record_file.exists():
        content = record_file.read_text(encoding="utf-8")
        lines = content.splitlines()
        if not lines:
            lines = RELEASE_RECORD_HEADER.splitlines()
    else:
        lines = RELEASE_RECORD_HEADER.splitlines()

    row_prefix = f"| `v{version}` |"
    for index, line in enumerate(lines):
        if line.startswith(row_prefix):
            if line == row:
                return False, note
            lines[index] = row
            record_file.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")
            return True, note

    lines.append(row)
    record_file.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")
    return True, note


def main() -> int:
    parser = argparse.ArgumentParser(description="Update Docker release history from cust commits")
    parser.add_argument("--repo", default=".", help="Repository path")
    parser.add_argument("--version", required=True, help="Release version without release/v prefix")
    parser.add_argument("--rev", default="HEAD", help="Commit or tag to inspect, defaults to HEAD")
    parser.add_argument("--date", default=date.today().isoformat(), help="Release date in YYYY-MM-DD")
    args = parser.parse_args()

    repo = Path(args.repo).resolve()
    require_repo(repo)
    changed, note = update_release_record(repo, args.version, args.date, args.rev)
    print(f"record: {RELEASE_RECORD_RELATIVE.as_posix()}")
    print(f"version: {args.version}")
    print(f"rev: {args.rev}")
    print(f"changed: {'yes' if changed else 'no'}")
    print(f"note: {note}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
