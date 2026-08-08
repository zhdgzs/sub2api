---
name: remote-docker
description: "Prepare a Sub2API cust release from the current development branch, push origin/cust, then let GitHub Actions build and push the official Docker image from the cust branch. Use when local release-docker builds are too slow and the release image should be built remotely by GitHub instead of local Docker."
---

# Remote Docker

## Scope

Use this skill only for release-mode Docker builds for this fork when the Docker build must run on GitHub Actions instead of the local machine. It follows the repository model in `docs/SYNC_UPSTREAM_CN.md`:

- `main` is only an upstream mirror.
- `cust` is the only formal Docker release build branch.
- The caller's current branch is treated as the source development branch.
- GitHub Actions builds from the pushed `cust` branch.

Do not modify or invoke `release-docker` execution mode for remote builds. This skill may reuse the existing release helper functions, but its script is the source of truth for the remote-build sequence.

## Safety Rules

This workflow performs high-risk operations: branch checkout, pull, merge, commit, push, and a remote Docker release build.

A direct user invocation of `$remote-docker` in the current turn is explicit approval for this entire release workflow, including the embedded `sync-upstream` step. Begin the workflow without asking whether to proceed. Do not request a separate `sync-upstream` confirmation or a second version confirmation when using the script's candidate version.

Still stop for a consolidated user decision when the read-only risk assessment finds text conflicts or high-risk shared edits requiring a product or code-semantic choice. This is a merge-resolution decision, not a release-start confirmation.

Do not auto-stash, auto-resolve conflicts, delete branches, delete tags, force-push, push to `upstream`, wait for the GitHub Actions build to finish, or call production deploy/restart endpoints manually.

Before every release merge, run the read-only risk assessment. Report all text conflicts and all high-risk shared edits that Git would auto-merge from both `main → cust` and `source → cust` in one response. For each path, state the recommended strategy and the required verification. When any recommendation requires a product or code-semantic choice, stop and request one consolidated user decision before modifying `cust`. In i18n files, explicitly check that object keys remain unique; a clean Git merge does not guarantee valid TypeScript.

Treat generated files as derived artifacts: resolve their source definitions first, regenerate when tooling is available, and never resolve a generated-file conflict by blindly choosing one side.

Use the candidate version from the post-sync preview by default. If the user supplied an explicit version in the initial request, use it only if it validates against the synced `main` base version. Do not use `--allow-version-override` unless that override was explicitly approved before the first mutating command; otherwise stop and report the validation error instead of starting a second confirmation round.

## Required Files

Remote builds require:

```text
.github/workflows/remote-docker.yml
.agents/skills/sync-upstream/scripts/sync_upstream.py
.agents/skills/release-docker/scripts/release_docker.py
.agents/skills/remote-docker/scripts/remote_docker.py
scripts/update_docker_release_record.py
/root/sub2api-deploy/watch_remote_docker_deploy.sh
```

The workflow builds the root `Dockerfile` from the pushed `cust` branch and pushes:

```text
ghcr.io/<owner>/sub2api:<version>
ghcr.io/<owner>/sub2api:cust
```

## Standard Workflow

1. Read `docs/SYNC_UPSTREAM_CN.md`.
2. Confirm the current directory is the intended `sub2api` repository.
3. Record the current branch as the source branch.
4. Require a clean working tree before starting.
5. Treat the direct `$remote-docker` invocation as approval and run `sync-upstream` first so `main` is current before choosing the release version:

   ```bash
   python3 ".agents/skills/sync-upstream/scripts/sync_upstream.py" --repo "$PWD" --yes
   ```

6. Run the remote release script in preview mode after the sync. Review its `merge_risk_assessment`, which reports conflicts on the actual release merge paths:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD"
   ```

7. Review the proposed source branch, `main`, `cust`, version candidate, GHCR tags, workflow file, release record file, and every conflict recommendation.
8. If conflicts exist, present the complete list with recommendations in one response and wait for a single user decision. Resolve the approved decisions, run applicable quality checks, and repeat the preview until no unresolved conflicts remain.
9. Choose the script's `candidate_version` unless the user already supplied an explicit valid version in the initial request.
10. Execute immediately after the conflict-free preview with the chosen version:

   ```bash
   python3 ".agents/skills/remote-docker/scripts/remote_docker.py" --repo "$PWD" --yes --version <chosen-version>
   ```

11. Report the release commit, GHCR image tags, workflow URL, deploy watcher PID, and deploy watcher log path.
12. Treat the task as complete after the workflow is triggered and the deploy watcher script is started. Do not wait for the watcher, poll Actions from the agent, or manually restart production.

## Script Contract

`scripts/remote_docker.py` is the deterministic sequence for this skill.

Preview mode must not mutate the repository.

Execution mode:

- Requires `--yes` and `--version`.
- Requires the initial source branch worktree to be clean.
- Calls the project `sync-upstream` script with `--yes`. A conflict-preview exit code is retained so the script can report all release-path conflicts.
- Checks out `cust`.
- Pulls `origin/cust` with `--ff-only` when the remote branch exists; the first release creates it with an upstream-tracking push.
- Merges `main` into `cust` with `--no-ff` when needed.
- Merges the recorded source branch into `cust` with `--no-ff` unless already contained.
- Performs a read-only risk assessment before checkout or merge. If any text conflict or high-risk auto-merged shared edit exists, prints every affected path with its recommendation and exits without modifying `cust`.
- Does not auto-resolve conflicts. After reviewed decisions are applied and the preview is conflict-free, normal merges create their own commits.
- Writes `backend/cmd/server/VERSION`.
- Writes/updates `docs/DOCKER_RELEASE_HISTORY.md`.
- Commits `chore(release): 发布 <version>` only if release metadata changed.
- Pushes `origin/cust`.
- Starts `/root/sub2api-deploy/watch_remote_docker_deploy.sh` in the background after the push, explicitly setting the `cust` branch/image variables and `EXPECTED_HEAD_SHA`.
- Does not run local Docker build.
- Relies on `.github/workflows/remote-docker.yml` to build and push GHCR images from `cust`.
- Does not wait for GitHub Actions or the deploy watcher to finish.

## Restart Policy

Remote Docker only triggers the remote build and starts the existing external deploy watcher. It must not configure deploy hooks, call restart endpoints, or assume any production deployment topology beyond invoking `/root/sub2api-deploy/watch_remote_docker_deploy.sh`.

The deploy watcher is responsible for polling GitHub Actions, pulling `ghcr.io/zhdgzs/sub2api:cust`, tagging `sub2api:cust`, running `docker compose up -d` in `/root/sub2api-deploy`, and sending Feishu notifications. The agent must not keep monitoring the build after this script is launched.

Manual `workflow_dispatch` is present for reruns, but `cust` branch push is the primary trigger because this fork keeps `main` as an upstream mirror.
