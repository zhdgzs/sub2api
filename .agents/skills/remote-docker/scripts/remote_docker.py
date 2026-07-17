#!/usr/bin/env python3
"""Prepare a Sub2API release and trigger a remote Docker build.

Preview mode is read-only. Execution requires --yes and --version.
"""

from __future__ import annotations

import argparse
import importlib.util
import os
import re
import subprocess
import sys
from datetime import date
from pathlib import Path
from types import ModuleType


REMOTE_WORKFLOW_RELATIVE = Path(".github/workflows/remote-docker.yml")
WORKFLOW_NAME = "remote-docker.yml"
DEPLOY_WATCHER = Path("/root/sub2api-deploy/watch_remote_docker_deploy.sh")
DEPLOY_WATCHER_LOG = Path("/root/sub2api-deploy/watch_remote_docker_deploy.log")


def load_release_module(repo: Path) -> ModuleType:
    script = repo / ".agents" / "skills" / "release-docker" / "scripts" / "release_docker.py"
    if not script.exists():
        raise SystemExit("[ERROR] release-docker helper script not found")

    spec = importlib.util.spec_from_file_location("sub2api_release_docker", script)
    if spec is None or spec.loader is None:
        raise SystemExit(f"[ERROR] Cannot load release helper: {script}")

    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def require_remote_workflow(repo: Path) -> None:
    workflow = repo / REMOTE_WORKFLOW_RELATIVE
    if not workflow.exists():
        raise SystemExit(f"[ERROR] {REMOTE_WORKFLOW_RELATIVE.as_posix()} not found")


def origin_slug(release: ModuleType, repo: Path) -> str | None:
    url = release.git(repo, "remote", "get-url", "origin", check=False).strip()
    patterns = [
        r"^git@github\.com:(?P<owner>[^/]+)/(?P<repo>[^/]+?)(?:\.git)?$",
        r"^https://github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+?)(?:\.git)?$",
    ]
    for pattern in patterns:
        match = re.match(pattern, url)
        if match:
            return f"{match.group('owner')}/{match.group('repo')}"
    return None


def image_name(release: ModuleType, repo: Path) -> str:
    slug = origin_slug(release, repo)
    if not slug:
        return "ghcr.io/<owner>/sub2api"
    owner, _ = slug.split("/", 1)
    return f"ghcr.io/{owner.lower()}/sub2api"


def actions_url(release: ModuleType, repo: Path) -> str | None:
    slug = origin_slug(release, repo)
    if not slug:
        return None
    return f"https://github.com/{slug}/actions/workflows/{WORKFLOW_NAME}"


def full_head(release: ModuleType, repo: Path) -> str:
    return release.git(repo, "rev-parse", "HEAD").strip()


def record_versions(release: ModuleType, repo: Path, base_version: str) -> list[str]:
    record_file = repo / release.RELEASE_RECORD_RELATIVE
    if not record_file.exists():
        return []
    pattern = re.compile(rf"`v({re.escape(base_version)}-zhdgzs\.\d+)`")
    return pattern.findall(record_file.read_text(encoding="utf-8"))


def current_file_version(repo: Path) -> str | None:
    version_file = repo / "backend" / "cmd" / "server" / "VERSION"
    if not version_file.exists():
        return None
    return version_file.read_text(encoding="utf-8").strip() or None


def remote_candidate_version(release: ModuleType, repo: Path, base_version: str) -> str:
    highest = 0
    versions = [tag.removeprefix("release/v") for tag in release.release_tags(repo, base_version)]
    versions.extend(record_versions(release, repo, base_version))
    current = current_file_version(repo)
    if current:
        versions.append(current)

    for raw in versions:
        match = release.VERSION_RE.match(raw)
        if match and match.group("base") == base_version:
            highest = max(highest, int(match.group("num")))
    return f"{base_version}-zhdgzs.{highest + 1}"


def merge_conflict_paths(release: ModuleType, repo: Path, target: str, incoming: str) -> list[str]:
    """Return paths Git reports as conflicted when merging incoming into target."""
    if target == incoming or release.branch_contains(repo, incoming, target):
        return []

    proc = release.run(
        ["git", "merge-tree", "--write-tree", "--messages", "--name-only", target, incoming],
        cwd=repo,
        check=False,
    )
    if proc.returncode == 0:
        return []
    if proc.returncode != 1:
        detail = (proc.stderr or proc.stdout or "").strip()
        raise SystemExit(f"[ERROR] Cannot assess merge {incoming} -> {target}.\n{detail}")

    paths: list[str] = []
    for raw in (proc.stdout + "\n" + proc.stderr).splitlines():
        match = re.search(r"^CONFLICT \([^)]*\): Merge conflict in (?P<path>.+)$", raw.strip())
        if match:
            paths.append(match.group("path"))
    return sorted(set(paths))


def merge_recommendation(path: str) -> str:
    name = Path(path).name
    if name == "wire_gen.go" or name.endswith("_gen.go"):
        return "以对应的 Wire/源定义为准，合并后重新生成；不要直接选任一生成文件。"
    if "__tests__/" in path or name.endswith("_test.go") or name.endswith(".spec.ts"):
        return "合并双方测试场景，确认旧行为与新增行为都被覆盖。"
    if name.endswith((".vue", ".go", ".ts", ".tsx")):
        return "人工组合双方改动，核对模板/类型/调用链或服务/生成代码的一致性。"
    if name.endswith((".lock", "pnpm-lock.yaml", "go.sum")):
        return "根据合并后的依赖清单重新生成，避免直接选边。"
    return "人工审查双方语义后决定，保留互不冲突的改动。"


def release_merge_risks(release: ModuleType, repo: Path, source_branch: str) -> list[tuple[str, str, list[str]]]:
    risks: list[tuple[str, str, list[str]]] = []
    if not release.branch_contains(repo, "main", "custom"):
        paths = merge_conflict_paths(release, repo, "custom", "main")
        if paths:
            risks.append(("main", "custom", paths))
    if source_branch != "custom" and not release.branch_contains(repo, source_branch, "custom"):
        paths = merge_conflict_paths(release, repo, "custom", source_branch)
        if paths:
            risks.append((source_branch, "custom", paths))
    return risks


def print_merge_risk_assessment(release: ModuleType, repo: Path, source_branch: str) -> list[tuple[str, str, list[str]]]:
    risks = release_merge_risks(release, repo, source_branch)
    print("merge_risk_assessment:")
    if not risks:
        print("- 未发现发布路径上的文本冲突；仍需执行发布前质量检查。")
        return risks

    print("- 发现需要决策的语义冲突；请一次性确认下列建议后再执行发布。")
    for incoming, target, paths in risks:
        print(f"- merge: {incoming} -> {target}")
        for path in paths:
            print(f"  - {path}: {merge_recommendation(path)}")
    return risks


def print_plan(release: ModuleType, state: object, version: str | None) -> None:
    candidate = remote_candidate_version(release, state.repo, state.main_base_version)
    chosen = version or candidate
    image = image_name(release, state.repo)
    url = actions_url(release, state.repo)
    workflow_status = "present" if (state.repo / REMOTE_WORKFLOW_RELATIVE).exists() else "missing"

    print("Remote Docker preview")
    print(f"repo: {state.repo}")
    print(f"release_record: {release.RELEASE_RECORD_RELATIVE.as_posix()}")
    print(f"workflow: {REMOTE_WORKFLOW_RELATIVE.as_posix()} ({workflow_status})")
    print(f"source_branch: {state.source_branch} ({state.head})")
    print(f"main: {state.main}")
    print(f"custom: {state.custom or 'missing'}")
    print(f"origin/custom: {state.origin_custom or 'missing'}")
    print(f"main_base_version: {state.main_base_version}")
    print(f"candidate_version: {candidate}")
    print(f"chosen_version: {chosen}")
    print(f"deploy_watcher: {DEPLOY_WATCHER}")
    print(f"deploy_watcher_log: {DEPLOY_WATCHER_LOG}")
    print("remote_docker_tags:")
    print(f"- {image}:{chosen}")
    print(f"- {image}:custom")
    if url:
        print(f"workflow_url: {url}")
    print_merge_risk_assessment(release, state.repo, state.source_branch)
    print()
    print("Execution sequence with --yes:")
    print("1. require clean working tree")
    print("2. run sync-upstream --yes")
    print("3. checkout custom")
    print("4. git pull --ff-only origin custom")
    print("5. merge --no-ff main when needed")
    print("6. merge --no-ff source branch if not already contained")
    print("7. update backend/cmd/server/VERSION")
    print(f"8. update {release.RELEASE_RECORD_RELATIVE.as_posix()} from custom commits only")
    print("9. commit release metadata if needed")
    print("10. push origin custom")
    print("11. GitHub Actions builds and pushes the Docker image from custom")
    print("12. start deploy watcher script in background and do not wait for it")


def start_deploy_watcher(release: ModuleType, repo: Path, head_sha: str) -> int:
    if not DEPLOY_WATCHER.exists():
        raise SystemExit(f"[ERROR] Deploy watcher script not found: {DEPLOY_WATCHER}")
    if not os.access(DEPLOY_WATCHER, os.R_OK):
        raise SystemExit(f"[ERROR] Deploy watcher script is not readable: {DEPLOY_WATCHER}")

    DEPLOY_WATCHER_LOG.parent.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    slug = origin_slug(release, repo)
    if slug:
        env.setdefault("REPO", slug)
    env.setdefault("WORKFLOW", WORKFLOW_NAME)
    env.setdefault("BRANCH", "custom")
    env["EXPECTED_HEAD_SHA"] = head_sha

    with DEPLOY_WATCHER_LOG.open("ab") as output:
        proc = subprocess.Popen(
            ["bash", str(DEPLOY_WATCHER)],
            cwd=repo,
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=output,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    return proc.pid


def run_sync_upstream_for_assessment(release: ModuleType, repo: Path) -> None:
    """Sync main; keep conflict-preview output for the release risk report."""
    script = repo / ".agents" / "skills" / "sync-upstream" / "scripts" / "sync_upstream.py"
    if not script.exists():
        raise SystemExit("[ERROR] sync-upstream script not found")

    proc = release.run(
        ["python3", str(script), "--repo", str(repo), "--yes"],
        cwd=repo,
        check=False,
        capture=False,
    )
    if proc.returncode == 0:
        return
    if proc.returncode == 2:
        print("[INFO] 检测到合并预检冲突；发布脚本会在修改 custom 前汇总并停止。")
        return
    raise SystemExit(f"[ERROR] sync-upstream failed with exit code {proc.returncode}")


def execute(release: ModuleType, repo: Path, version: str, allow_version_override: bool) -> None:
    release.require_clean(repo)
    source_branch = release.current_branch(repo)
    run_sync_upstream_for_assessment(release, repo)
    base = release.main_version(repo)
    release.validate_version(version, base, allow_override=allow_version_override)

    risks = print_merge_risk_assessment(release, repo, source_branch)
    if risks:
        raise SystemExit("[ERROR] Release merge conflicts require reviewed decisions before modifying custom.")

    release.git(repo, "checkout", "custom")
    release.git(repo, "pull", "--ff-only", "origin", "custom")

    if not release.branch_contains(repo, "main", "custom"):
        release.merge_branch(repo, "main", f"chore: 合并上游 main {base}")

    if source_branch != "custom" and not release.branch_contains(repo, source_branch, "custom"):
        release.merge_branch(repo, source_branch, f"chore: 合并 {source_branch} 到 custom")

    require_remote_workflow(repo)
    release.require_clean_or_merge_complete(repo)
    release_date = date.today().isoformat()
    version_changed = release.write_version(repo, version)
    record_changed, note = release.update_release_record(repo, version, release_date)
    if version_changed or record_changed:
        release.git(repo, "add", "backend/cmd/server/VERSION", str(release.RELEASE_RECORD_RELATIVE))
        release.git(repo, "commit", "-m", f"chore(release): {version}")
    else:
        print(f"[INFO] Release metadata already up to date for {version}; no release commit needed")

    head_sha = full_head(release, repo)
    release.git(repo, "push", "origin", "custom")
    watcher_pid = start_deploy_watcher(release, repo, head_sha)

    commit = release.short(repo, "HEAD")
    image = image_name(release, repo)
    url = actions_url(release, repo)
    print("Remote Docker release trigger complete")
    print(f"record: {release.RELEASE_RECORD_RELATIVE.as_posix()}")
    print(f"version: {version}")
    print(f"commit: {commit}")
    print(f"docker: {image}:{version}, {image}:custom")
    if url:
        print(f"workflow: {url}")
    print(f"deploy_watcher_pid: {watcher_pid}")
    print(f"deploy_watcher_log: {DEPLOY_WATCHER_LOG}")
    print(f"note: {note}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Prepare Sub2API release metadata for remote Docker build")
    parser.add_argument("--repo", default=".", help="Repository path")
    parser.add_argument("--yes", action="store_true", help="Execute mutating remote release workflow")
    parser.add_argument("--version", help="Confirmed release version, e.g. 0.1.142-zhdgzs.2")
    parser.add_argument(
        "--allow-version-override",
        action="store_true",
        help="Allow version base that differs from main VERSION; requires explicit user confirmation",
    )
    args = parser.parse_args()

    repo = Path(args.repo).resolve()
    release = load_release_module(repo)
    release.require_repo(repo)

    if args.yes and not args.version:
        raise SystemExit("[ERROR] --yes requires --version")

    state = release.load_state(repo)
    if not args.yes:
        print_plan(release, state, args.version)
        if args.version:
            release.validate_version(args.version, state.main_base_version, allow_override=args.allow_version_override)
        return 0

    execute(release, repo, args.version, args.allow_version_override)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
