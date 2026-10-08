"""Execute the actual promotion shell against isolated Git remotes."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
GIT = shutil.which("git")


def promotion_shell():
    source = (ROOT / ".github/workflows/promote-fork.yml").read_text()
    step = source.split("      - name: Promote generated fork\n", 1)[1]
    run = step.split("        run: |\n", 1)[1]
    return "\n".join(line[10:] if line.startswith("          ") else line for line in run.splitlines())


class PromotionTransactionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="fork-maintenance-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.upstream, self.remote, self.worker = (self.root / name for name in ("upstream", "remote.git", "worker"))
        name = subprocess.check_output([GIT, "log", "-1", "--format=%an"], cwd=ROOT, text=True).strip()
        email = subprocess.check_output([GIT, "log", "-1", "--format=%ae"], cwd=ROOT, text=True).strip()
        self.env = dict(os.environ, GIT_AUTHOR_NAME=name, GIT_AUTHOR_EMAIL=email,
                        GIT_COMMITTER_NAME=name, GIT_COMMITTER_EMAIL=email,
                        GIT_CONFIG_GLOBAL=os.devnull, GIT_ALLOW_PROTOCOL="file",
                        GIT_TERMINAL_PROMPT="0")
        self.upstream.mkdir()
        self.git(self.upstream, "init", "-b", "main")
        version = self.upstream / "backend/cmd/server/VERSION"
        version.parent.mkdir(parents=True)
        version.write_text("1.0.0\n")
        self.commit(self.upstream, "release one")
        self.anchor = self.git(self.upstream, "rev-parse", "HEAD")
        self.git(self.upstream, "tag", "v1.0.0")
        version.write_text("2.0.0\n")
        self.commit(self.upstream, "release two")
        self.target = self.git(self.upstream, "rev-parse", "HEAD")
        self.git(self.upstream, "tag", "v2.0.0")
        self.git(self.root, "clone", "--bare", str(self.upstream), str(self.remote))
        self.git(self.root, "clone", str(self.remote), str(self.worker))
        self.git(self.worker, "config", f"url.{self.upstream}.insteadOf", "https://github.com/Wei-Shaw/sub2api.git")
        self.git(self.worker, "checkout", "-b", "fork", self.anchor)
        (self.worker / "feature.txt").write_text("fork feature\n")
        self.commit(self.worker, "fork feature")
        self.base = self.git(self.worker, "rev-parse", "HEAD")
        self.branch = "sync/upstream-v2.0.0"
        self.git(self.worker, "checkout", "-b", self.branch, self.target)
        self.git(self.worker, "cherry-pick", self.base)
        self.head = self.git(self.worker, "rev-parse", "HEAD")
        self.git(self.worker, "checkout", "-b", "concurrent", self.base)
        (self.worker / "concurrent.txt").write_text("concurrent feature\n")
        self.commit(self.worker, "concurrent change")
        self.race_sha = self.git(self.worker, "rev-parse", "HEAD")
        self.git(self.worker, "push", "origin", f"{self.base}:refs/heads/fork",
                 f"{self.anchor}:refs/heads/upstream-release", f"{self.head}:refs/heads/{self.branch}",
                 "concurrent:refs/heads/concurrent")
        self.git(self.worker, "checkout", "fork")
        policy = self.worker / ".github/scripts/fork_promotion.py"
        policy.parent.mkdir(parents=True)
        shutil.copyfile(ROOT / ".github/scripts/fork_promotion.py", policy)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text('''#!/bin/sh
case "$1 $2" in
 "issue list") echo "[]";;
 "pr view") echo CLOSED;;
 api*) printf '{"tag_name":"v2.0.0","published_at":"2026-10-08T08:00:00Z","draft":%s,"prerelease":false}\\n' "${FIXTURE_RELEASE_DRAFT:-false}";;
esac
''')
        gh.chmod(0o755)
        self.env.update(PATH=str(self.bin) + os.pathsep + self.env["PATH"],
                        REAL_GIT=GIT, FIXTURE_REMOTE=str(self.remote), RACE_SHA=self.race_sha,
                        BASE_BRANCH="fork", ANCHOR_BRANCH="upstream-release", HEAD_REF=self.branch,
                        HEAD_SHA=self.head, TAG="v2.0.0", SOURCE_BASE_SHA=self.base,
                        SOURCE_ANCHOR_SHA=self.anchor, SOURCE_EFFECTIVE_BASE_SHA=self.target, SOURCE_TAG_SHA=self.target,
                        UPSTREAM_REPOSITORY="Wei-Shaw/sub2api", GITHUB_REPOSITORY="fixture/sub2api",
                        PR_NUMBER="1", CONFLICT_ISSUE_LABEL="sync-upstream-conflict")

    def git(self, cwd, *args):
        return subprocess.check_output([GIT, *args], cwd=cwd, env=self.env, text=True, stderr=subprocess.PIPE).strip()

    def commit(self, cwd, message):
        self.git(cwd, "add", ".")
        self.git(cwd, "commit", "-m", message)

    def ref(self, branch):
        return self.git(self.remote, "rev-parse", f"refs/heads/{branch}")

    def run_promotion(self):
        return subprocess.run(["bash", "-c", promotion_shell()], cwd=self.worker, env=self.env,
                              text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)

    def inject_race(self, ref, cleanup=False):
        self.env["RACE_REF"] = f"refs/heads/{ref}"
        wrapper = self.bin / "git"
        condition = '[[ "$1" == push && "$2" == --force-with-lease=* ]]' if cleanup else '[[ "$1" == push && "$2" == --atomic ]]'
        wrapper.write_text(f'''#!/bin/bash
if {condition}; then
  "$REAL_GIT" -C "$FIXTURE_REMOTE" update-ref "$RACE_REF" "$RACE_SHA"
fi
exec "$REAL_GIT" "$@"
''')
        wrapper.chmod(0o755)

    def test_promotes_both_production_refs_and_removes_candidate(self):
        result = self.run_promotion()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.ref("fork"), self.head)
        self.assertEqual(self.ref("upstream-release"), self.target)
        refs = self.git(self.remote, "for-each-ref", "--format=%(refname)")
        self.assertNotIn("refs/heads/" + self.branch, refs)

    def test_stale_source_is_rejected_before_any_production_write(self):
        self.git(self.remote, "update-ref", "refs/heads/fork", self.race_sha)
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.ref("fork"), self.race_sha)
        self.assertEqual(self.ref("upstream-release"), self.anchor)
        self.assertEqual(self.ref(self.branch), self.head)

    def test_unpublished_release_is_rejected_before_any_production_write(self):
        self.env["FIXTURE_RELEASE_DRAFT"] = "true"
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.ref("fork"), self.base)
        self.assertEqual(self.ref("upstream-release"), self.anchor)

    def test_changed_upstream_tag_is_rejected_before_any_production_write(self):
        self.git(self.worker, "push", "--force", str(self.upstream), f"{self.race_sha}:refs/tags/v2.0.0")
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.ref("fork"), self.base)
        self.assertEqual(self.ref("upstream-release"), self.anchor)

    def test_atomic_push_refuses_concurrent_source_update_without_partial_promotion(self):
        self.inject_race("fork")
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.ref("fork"), self.race_sha)
        self.assertEqual(self.ref("upstream-release"), self.anchor)
        self.assertEqual(self.ref(self.branch), self.head)

    def test_concurrent_candidate_update_prevents_production_promotion(self):
        self.inject_race(self.branch)
        result = self.run_promotion()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.ref("fork"), self.base)
        self.assertEqual(self.ref("upstream-release"), self.anchor)
        self.assertEqual(self.ref(self.branch), self.race_sha)

    def test_cleanup_does_not_delete_a_candidate_that_moved(self):
        self.inject_race(self.branch, cleanup=True)
        result = self.run_promotion()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.ref("fork"), self.head)
        self.assertEqual(self.ref("upstream-release"), self.target)
        self.assertEqual(self.ref(self.branch), self.race_sha)

    def test_already_promoted_candidate_only_finishes_cleanup(self):
        self.git(self.remote, "update-ref", "refs/heads/fork", self.head)
        self.git(self.remote, "update-ref", "refs/heads/upstream-release", self.target)
        self.inject_race("fork")
        result = self.run_promotion()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("already promoted", result.stdout)
        self.assertEqual(self.ref("fork"), self.head)
        self.assertEqual(self.ref("upstream-release"), self.target)


if __name__ == "__main__":
    unittest.main()
