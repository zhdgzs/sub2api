#!/usr/bin/env python3
"""Prepare a Sub2API release and trigger a remote Docker build.

Preview mode is read-only. Execution requires --yes and --version.
"""

from __future__ import annotations

import argparse
import importlib.util
import re
import sys
from datetime import date
from pathlib import Path
from types import ModuleType


REMOTE_WORKFLOW_RELATIVE = Path(".github/workflows/remote-docker.yml")
WORKFLOW_NAME = "remote-docker.yml"


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


def print_plan(release: ModuleType, state: object, version: str | None) -> None:
    chosen = version or state.candidate_version
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
    print(f"candidate_version: {state.candidate_version}")
    print(f"chosen_version: {chosen}")
    print(f"release_tag: release/v{chosen}")
    print("remote_docker_tags:")
    print(f"- {image}:{chosen}")
    print(f"- {image}:custom")
    if url:
        print(f"workflow_url: {url}")
    print("deploy_hook: optional secret REMOTE_DOCKER_WEBHOOK_URL")
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
    print("11. create and push annotated release tag")
    print("12. GitHub Actions builds and pushes the Docker image from the tag")


def execute(release: ModuleType, repo: Path, version: str, allow_version_override: bool) -> None:
    release.require_clean(repo)
    source_branch = release.current_branch(repo)
    release.run_sync_upstream(repo)
    base = release.main_version(repo)
    release.validate_version(version, base, allow_override=allow_version_override)
    tag = f"release/v{version}"
    if release.tag_exists(repo, tag):
        raise SystemExit(f"[ERROR] Tag already exists: {tag}")

    release.git(repo, "checkout", "custom")
    release.git(repo, "pull", "--ff-only", "origin", "custom")

    if not release.branch_contains(repo, "main", "custom"):
        release.merge_branch(repo, "main", f"chore: merge upstream main {base}")

    if source_branch != "custom" and not release.branch_contains(repo, source_branch, "custom"):
        release.merge_branch(repo, source_branch, f"chore: merge {source_branch} into custom")

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

    release.git(repo, "push", "origin", "custom")
    release.git(repo, "tag", "-a", tag, "-m", tag)
    release.git(repo, "push", "origin", tag)

    commit = release.short(repo, "HEAD")
    image = image_name(release, repo)
    url = actions_url(release, repo)
    print("Remote Docker release trigger complete")
    print(f"record: {release.RELEASE_RECORD_RELATIVE.as_posix()}")
    print(f"version: {version}")
    print(f"tag: {tag}")
    print(f"commit: {commit}")
    print(f"docker: {image}:{version}, {image}:custom")
    if url:
        print(f"workflow: {url}")
    print("deploy_hook: GitHub Actions will call REMOTE_DOCKER_WEBHOOK_URL after a successful image push if configured")
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
