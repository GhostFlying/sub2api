import copy
import json
import unittest

from fork_promotion import inspect_candidate, select_release


REPOSITORY = "GhostFlying/sub2api"
HEAD, BASE, ANCHOR, UPSTREAM = (letter * 40 for letter in "abcd")
REQUIRED = ["test", "frontend", "golangci-lint", "fork-maintenance", "backend-security", "frontend-security"]


def candidate():
    return {
        "number": 10, "state": "open", "draft": True,
        "head": {"ref": "sync/upstream-v2.0.0", "sha": HEAD, "repo": {"full_name": REPOSITORY}},
        "base": {"ref": "fork", "sha": BASE, "repo": {"full_name": REPOSITORY}},
        "body": '<!-- fork-sync-source ' + json.dumps({"version": 2, "tag": "v2.0.0",
                 "head_sha": HEAD, "base_sha": BASE, "anchor_sha": ANCHOR,
                 "effective_base_sha": UPSTREAM, "tag_sha": UPSTREAM}) + ' -->',
    }


def passing_checks():
    return [{"id": index, "name": name, "head_sha": HEAD, "status": "completed", "conclusion": "success",
             "started_at": "2026-10-08T08:00:00Z", "app": {"slug": "github-actions"}}
            for index, name in enumerate(REQUIRED)]


class ReleaseSelectionTests(unittest.TestCase):
    def releases(self):
        return [{"tag_name": tag, "published_at": "2026-10-08T08:00:00Z", "draft": False, "prerelease": False}
                for tag in ("v1.9.0", "v1.10.0", "v1.8.99", "v2.0.0-rc.1", "mobile-v9.0.0")]

    def test_selects_highest_stable_version_instead_of_publication_order(self):
        self.assertEqual(select_release(self.releases()), "v1.10.0")

    def test_ignores_drafts_prereleases_and_unpublished_versions(self):
        releases = self.releases() + [
            {"tag_name": "v8.0.0", "published_at": "today", "draft": True},
            {"tag_name": "v7.0.0", "published_at": "today", "prerelease": True},
            {"tag_name": "v6.0.0", "published_at": None},
        ]
        self.assertEqual(select_release(releases), "v1.10.0")

    def test_explicit_requests_use_the_same_eligibility_rules(self):
        self.assertEqual(select_release(self.releases(), "v1.9.0"), "v1.9.0")
        for tag in ("v2.0.0-rc.1", "mobile-v9.0.0", "v3.0.0", "v01.9.0", "$(command)"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                select_release(self.releases(), tag)


class CandidatePolicyTests(unittest.TestCase):
    def inspect(self, pr=None, checks=None, expected_head=HEAD):
        return inspect_candidate(pr or candidate(), checks if checks is not None else passing_checks(),
                                 REPOSITORY, REQUIRED, expected_head)

    def test_passing_draft_candidate_needs_no_human_prompt(self):
        result = self.inspect()
        self.assertEqual(result["ready"], "true")
        self.assertEqual(result["head_sha"], HEAD)
        self.assertEqual(result["source_effective_base_sha"], UPSTREAM)

    def test_missing_or_running_checks_wait_for_next_completed_event(self):
        self.assertEqual(self.inspect(checks=[])["result"], "waiting")
        checks = passing_checks()
        checks[-1].update(status="in_progress", conclusion=None)
        result = self.inspect(checks=checks)
        self.assertEqual(result["pending"], "frontend-security")
        self.assertEqual(result["ready"], "false")

    def test_failure_cancellation_skipped_and_neutral_cannot_promote(self):
        for conclusion in ("failure", "cancelled", "timed_out", "skipped", "neutral", None):
            with self.subTest(conclusion=conclusion):
                checks = passing_checks()
                checks[-1]["conclusion"] = conclusion
                self.assertEqual(self.inspect(checks=checks)["result"], "blocked")

    def test_newest_retry_supersedes_old_check_and_waits_while_running(self):
        checks = passing_checks()
        checks[-1]["conclusion"] = "failure"
        retry = dict(checks[-1], id=100, started_at="2026-10-08T08:01:00Z", status="in_progress")
        checks.append(retry)
        self.assertEqual(self.inspect(checks=checks)["result"], "waiting")
        retry.update(status="completed", conclusion="success")
        self.assertEqual(self.inspect(checks=checks)["ready"], "true")
        retry.update(conclusion="failure")
        self.assertEqual(self.inspect(checks=checks)["result"], "blocked")

    def test_other_shas_or_apps_cannot_satisfy_required_checks(self):
        for changes in ({"head_sha": BASE}, {"app": {"slug": "external-ci"}}, {"app": {}}):
            with self.subTest(changes=changes):
                checks = passing_checks()
                checks[-1].update(changes)
                self.assertEqual(self.inspect(checks=checks)["missing"], "frontend-security")

    def test_foreign_heads_and_wrong_target_or_branch_are_rejected(self):
        for section, changes in (
            ("head", {"repo": {"full_name": "someone/sub2api"}}), ("head", {"repo": None}),
            ("base", {"ref": "main"}), ("head", {"ref": "feature/example"}),
            ("head", {"ref": "sync/upstream-v2.0.0-rc.1"}),
        ):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                pr = candidate()
                pr[section].update(changes)
                self.inspect(pr=pr)

    def test_marker_must_be_unique_and_bind_every_ref(self):
        original = candidate()
        for body in ("", original["body"] + "\n" + original["body"], original["body"].replace(HEAD, BASE),
                     original["body"].replace('"version": 2', '"version": true'),
                     original["body"].replace(UPSTREAM, "invalid"), '<!-- fork-sync-source [] -->'):
            with self.subTest(body=body), self.assertRaises(ValueError):
                pr = copy.deepcopy(original)
                pr["body"] = body
                self.inspect(pr=pr)

    def test_stale_event_or_source_base_is_a_no_op(self):
        self.assertEqual(self.inspect(expected_head=BASE)["result"], "stale")
        pr = candidate()
        pr["base"]["sha"] = "e" * 40
        self.assertEqual(self.inspect(pr=pr)["result"], "stale")

    def test_closed_candidate_is_a_no_op_and_promoted_open_candidate_can_finish_cleanup(self):
        pr = candidate()
        pr["state"] = "closed"
        self.assertEqual(self.inspect(pr=pr)["result"], "closed")
        pr.update(state="open")
        pr["base"]["sha"] = HEAD
        self.assertEqual(self.inspect(pr=pr)["ready"], "true")


if __name__ == "__main__":
    unittest.main()
