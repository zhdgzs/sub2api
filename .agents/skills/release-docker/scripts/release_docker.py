#!/usr/bin/env python3
"""Prepare and build a Sub2API release Docker image.

Preview mode is read-only. Execution requires --yes and --version.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path


VERSION_RE = re.compile(r"^(?P<base>\d+\.\d+\.\d+)-zhdgzs\.(?P<num>\d+)$")


@dataclass(frozen=True)
class RepoState:
    repo: Path
    source_branch: str
    head: str
    main: str
    custom: str | None
    origin_custom: str | None
    main_base_version: str
    candidate_version: str
    release_tag: str


def run(
    args: list[str],
    *,
    cwd: Path,
    check: bool = True,
    capture: bool = True,
) -> subprocess.CompletedProcess[str]:
    proc = subprocess.run(
        args,
        cwd=cwd,
        check=False,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    )
    if check and proc.returncode != 0:
        cmd = " ".join(args)
        stderr = (proc.stderr or "").strip()
        stdout = (proc.stdout or "").strip()
        detail = stderr or stdout or f"exit code {proc.returncode}"
        raise SystemExit(f"[ERROR] {cmd}\n{detail}")
    return proc


def git(repo: Path, *args: str, check: bool = True) -> str:
    return run(["git", *args], cwd=repo, check=check).stdout.strip()


def short(repo: Path, rev: str) -> str:
    return git(repo, "rev-parse", "--short", rev)


def require_repo(repo: Path) -> None:
    if not (repo / ".git").exists():
        raise SystemExit(f"[ERROR] Not a git repository: {repo}")
    if not (repo / "docs" / "SYNC_UPSTREAM_CN.md").exists():
        raise SystemExit("[ERROR] docs/SYNC_UPSTREAM_CN.md not found")
    if not (repo / "backend" / "cmd" / "server" / "VERSION").exists():
        raise SystemExit("[ERROR] backend/cmd/server/VERSION not found")


def require_clean(repo: Path) -> None:
    status = git(repo, "status", "--porcelain")
    if status:
        raise SystemExit(
            "[ERROR] Working tree is not clean. Commit or discard changes before release.\n"
            + status
        )


def current_branch(repo: Path) -> str:
    branch = git(repo, "branch", "--show-current")
    if not branch:
        raise SystemExit("[ERROR] Detached HEAD is not supported for release Docker")
    return branch


def rev_or_none(repo: Path, rev: str) -> str | None:
    proc = run(["git", "rev-parse", "--verify", rev], cwd=repo, check=False)
    if proc.returncode != 0:
        return None
    return proc.stdout.strip()


def main_version(repo: Path) -> str:
    return git(repo, "show", "main:backend/cmd/server/VERSION").strip()


def release_tags(repo: Path, base_version: str) -> list[str]:
    pattern = f"release/v{base_version}-zhdgzs.*"
    output = git(repo, "tag", "--list", pattern)
    return [line.strip() for line in output.splitlines() if line.strip()]


def candidate_version(repo: Path, base_version: str) -> str:
    highest = 0
    for tag in release_tags(repo, base_version):
        raw = tag.removeprefix("release/v")
        match = VERSION_RE.match(raw)
        if match and match.group("base") == base_version:
            highest = max(highest, int(match.group("num")))
    return f"{base_version}-zhdgzs.{highest + 1}"


def validate_version(version: str, base_version: str, *, allow_override: bool) -> None:
    match = VERSION_RE.match(version)
    if not match:
        raise SystemExit(f"[ERROR] Invalid version: {version}. Expected <base>-zhdgzs.<n>")
    if match.group("base") != base_version and not allow_override:
        raise SystemExit(
            f"[ERROR] Version base {match.group('base')} does not match main base {base_version}. "
            "Use --allow-version-override only after explicit user confirmation."
        )


def tag_exists(repo: Path, tag: str) -> bool:
    return rev_or_none(repo, f"refs/tags/{tag}") is not None


def branch_contains(repo: Path, ancestor: str, descendant: str) -> bool:
    proc = run(["git", "merge-base", "--is-ancestor", ancestor, descendant], cwd=repo, check=False)
    return proc.returncode == 0


def load_state(repo: Path) -> RepoState:
    source = current_branch(repo)
    base = main_version(repo)
    candidate = candidate_version(repo, base)
    return RepoState(
        repo=repo,
        source_branch=source,
        head=short(repo, "HEAD"),
        main=short(repo, "main"),
        custom=short(repo, "custom") if rev_or_none(repo, "custom") else None,
        origin_custom=short(repo, "origin/custom") if rev_or_none(repo, "origin/custom") else None,
        main_base_version=base,
        candidate_version=candidate,
        release_tag=f"release/v{candidate}",
    )


def print_plan(state: RepoState, version: str | None) -> None:
    chosen = version or state.candidate_version
    print("Release Docker preview")
    print(f"repo: {state.repo}")
    print(f"source_branch: {state.source_branch} ({state.head})")
    print(f"main: {state.main}")
    print(f"custom: {state.custom or 'missing'}")
    print(f"origin/custom: {state.origin_custom or 'missing'}")
    print(f"main_base_version: {state.main_base_version}")
    print(f"candidate_version: {state.candidate_version}")
    print(f"chosen_version: {chosen}")
    print(f"release_tag: release/v{chosen}")
    print("docker_tags:")
    print(f"- sub2api:{chosen}")
    print("- sub2api:custom")
    print()
    print("Execution sequence with --yes:")
    print("1. require clean working tree")
    print("2. run sync-upstream --yes")
    print("3. checkout custom")
    print("4. git pull --ff-only origin custom")
    print("5. merge --no-ff main")
    print("6. merge --no-ff source branch if not already contained")
    print("7. update backend/cmd/server/VERSION")
    print("8. commit version if needed")
    print("9. push origin custom")
    print("10. create and push annotated release tag")
    print("11. docker build release tags")


def run_sync_upstream(repo: Path) -> None:
    script = repo / ".agents" / "skills" / "sync-upstream" / "scripts" / "sync_upstream.py"
    if not script.exists():
        raise SystemExit("[ERROR] sync-upstream script not found")
    run(["python3", str(script), "--repo", str(repo), "--yes"], cwd=repo, check=True, capture=False)


def merge_branch(repo: Path, branch: str, message: str) -> None:
    proc = run(["git", "merge", "--no-ff", branch, "-m", message], cwd=repo, check=False)
    if proc.returncode != 0:
        conflicts = git(repo, "diff", "--name-only", "--diff-filter=U", check=False)
        detail = conflicts or (proc.stderr or proc.stdout or "").strip()
        raise SystemExit(
            f"[ERROR] Merge failed for {branch}. Resolve conflicts, complete the merge, then rerun.\n{detail}"
        )


def write_version(repo: Path, version: str) -> bool:
    version_file = repo / "backend" / "cmd" / "server" / "VERSION"
    current = version_file.read_text(encoding="utf-8").strip()
    if current == version:
        return False
    version_file.write_text(version + "\n", encoding="utf-8")
    return True


def execute(repo: Path, version: str, allow_version_override: bool) -> None:
    require_clean(repo)
    source_branch = current_branch(repo)
    run_sync_upstream(repo)
    base = main_version(repo)
    validate_version(version, base, allow_override=allow_version_override)
    tag = f"release/v{version}"
    if tag_exists(repo, tag):
        raise SystemExit(f"[ERROR] Tag already exists: {tag}")

    git(repo, "checkout", "custom")
    git(repo, "pull", "--ff-only", "origin", "custom")

    if not branch_contains(repo, "main", "custom"):
        merge_branch(repo, "main", f"chore: merge upstream main {base}")

    if source_branch != "custom" and not branch_contains(repo, source_branch, "custom"):
        merge_branch(repo, source_branch, f"chore: merge {source_branch} into custom")

    require_clean_or_merge_complete(repo)
    changed = write_version(repo, version)
    if changed:
        git(repo, "add", "backend/cmd/server/VERSION")
        git(repo, "commit", "-m", f"chore(release): {version}")
    else:
        print(f"[INFO] VERSION already equals {version}; no version commit needed")

    git(repo, "push", "origin", "custom")
    git(repo, "tag", "-a", tag, "-m", tag)
    git(repo, "push", "origin", tag)

    commit = short(repo, "HEAD")
    run(
        [
            "docker",
            "build",
            "--build-arg",
            f"VERSION={version}",
            "--build-arg",
            f"COMMIT={commit}",
            "-t",
            f"sub2api:{version}",
            "-t",
            "sub2api:custom",
            ".",
        ],
        cwd=repo,
        check=True,
        capture=False,
    )
    print("Release Docker build complete")
    print(f"version: {version}")
    print(f"tag: {tag}")
    print(f"commit: {commit}")
    print(f"docker: sub2api:{version}, sub2api:custom")


def require_clean_or_merge_complete(repo: Path) -> None:
    conflicts = git(repo, "diff", "--name-only", "--diff-filter=U", check=False)
    if conflicts:
        raise SystemExit(f"[ERROR] Unresolved merge conflicts:\n{conflicts}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Prepare/build Sub2API release Docker image")
    parser.add_argument("--repo", default=".", help="Repository path")
    parser.add_argument("--yes", action="store_true", help="Execute mutating release workflow")
    parser.add_argument("--version", help="Confirmed release version, e.g. 0.1.133-zhdgzs.1")
    parser.add_argument(
        "--allow-version-override",
        action="store_true",
        help="Allow version base that differs from main VERSION; requires explicit user confirmation",
    )
    args = parser.parse_args()

    repo = Path(args.repo).resolve()
    require_repo(repo)

    if args.yes and not args.version:
        raise SystemExit("[ERROR] --yes requires --version")

    state = load_state(repo)
    if not args.yes:
        print_plan(state, args.version)
        if args.version:
            validate_version(args.version, state.main_base_version, allow_override=args.allow_version_override)
        return 0

    execute(repo, args.version, args.allow_version_override)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
