#!/usr/bin/env python3
"""Release selection and fail-closed policy for automatic fork promotion."""

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import sys


STABLE_TAG = re.compile(r"^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
SHA = re.compile(r"^[0-9a-f]{40}$")
MARKER = re.compile(r"^<!-- fork-sync-source (.+) -->$", re.MULTILINE)


def select_release(releases, requested=""):
    eligible = [
        release for release in releases
        if not release.get("draft") and not release.get("prerelease")
        and release.get("published_at") and STABLE_TAG.fullmatch(release.get("tag_name", ""))
    ]
    if requested:
        eligible = [release for release in eligible if release["tag_name"] == requested]
    if not eligible:
        raise ValueError("No matching published stable vX.Y.Z upstream Release")
    return max(eligible, key=lambda release: tuple(
        int(part) for part in STABLE_TAG.fullmatch(release["tag_name"]).groups()
    ))["tag_name"]


def check_order(check):
    timestamp = check.get("started_at") or check.get("completed_at")
    started = datetime.fromisoformat(timestamp.replace("Z", "+00:00")) if timestamp else datetime.min.replace(tzinfo=timezone.utc)
    return started, check.get("id", 0)


def inspect_candidate(pr, checks, repository, required_checks, expected_head=""):
    if pr.get("state") != "open":
        return {"result": "closed", "ready": "false"}
    head, base = pr.get("head", {}), pr.get("base", {})
    if (head.get("repo") or {}).get("full_name") != repository or (base.get("repo") or {}).get("full_name") != repository:
        raise ValueError("Only same-repository candidates may be promoted")
    branch, head_sha = head.get("ref", ""), head.get("sha", "")
    tag = branch.removeprefix("sync/upstream-")
    if base.get("ref") != "fork" or not branch.startswith("sync/upstream-") or not STABLE_TAG.fullmatch(tag):
        raise ValueError("Expected a stable sync/upstream-vX.Y.Z -> fork candidate")
    if not SHA.fullmatch(head_sha):
        raise ValueError("Invalid candidate SHA")
    if expected_head and expected_head != head_sha:
        return {"result": "stale", "ready": "false"}
    markers = MARKER.findall(pr.get("body") or "")
    if len(markers) != 1:
        raise ValueError("Expected exactly one fork-sync-source marker")
    source = json.loads(markers[0])
    if not isinstance(source, dict) or type(source.get("version")) is not int or source["version"] != 2:
        raise ValueError("Invalid source marker version")
    if source.get("tag") != tag or source.get("head_sha") != head_sha:
        raise ValueError("Source marker does not match the candidate")
    for key in ("base_sha", "anchor_sha", "effective_base_sha", "tag_sha"):
        if not SHA.fullmatch(source.get(key, "")):
            raise ValueError(f"Invalid source marker {key}")
    if base.get("sha") not in (source["base_sha"], head_sha):
        return {"result": "stale", "ready": "false"}

    latest = {}
    for check in checks:
        if check.get("head_sha") != head_sha or check.get("app", {}).get("slug") != "github-actions":
            continue
        name = check.get("name")
        if name not in latest or check_order(check) > check_order(latest[name]):
            latest[name] = check
    missing, pending, failed = [], [], []
    for name in required_checks:
        check = latest.get(name)
        if check is None:
            missing.append(name)
        elif check.get("status") != "completed":
            pending.append(name)
        elif check.get("conclusion") != "success":
            failed.append(name)
    if missing or pending or failed:
        return {"result": "blocked" if failed else "waiting", "ready": "false",
                "missing": ",".join(missing), "pending": ",".join(pending), "failed": ",".join(failed)}
    return {
        "result": "ready", "ready": "true", "pr_number": str(pr["number"]),
        "head_ref": branch, "head_sha": head_sha, "tag": tag,
        "source_base_sha": source["base_sha"], "source_anchor_sha": source["anchor_sha"],
        "source_effective_base_sha": source["effective_base_sha"], "source_tag_sha": source["tag_sha"],
    }


def read_jsonl(path):
    return [json.loads(line) for line in Path(path).read_text().splitlines() if line.strip()]


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    release = commands.add_parser("select-release")
    release.add_argument("--releases", required=True)
    release.add_argument("--requested", default="")
    candidate = commands.add_parser("inspect")
    candidate.add_argument("--pr", required=True)
    candidate.add_argument("--checks", required=True)
    candidate.add_argument("--repository", required=True)
    candidate.add_argument("--required-checks", required=True)
    candidate.add_argument("--expected-head", default="")
    args = parser.parse_args()
    try:
        if args.command == "select-release":
            print(select_release(read_jsonl(args.releases), args.requested))
            return 0
        result = inspect_candidate(json.loads(Path(args.pr).read_text()), read_jsonl(args.checks),
                                   args.repository, args.required_checks.split(), args.expected_head)
        print(json.dumps(result, sort_keys=True))
        output = os.environ.get("GITHUB_OUTPUT")
        if output:
            with open(output, "a", encoding="utf-8") as stream:
                for key, value in result.items():
                    stream.write(f"{key}={value}\n")
        return 0
    except (ValueError, TypeError, KeyError) as error:
        print(f"Refusing fork promotion: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
