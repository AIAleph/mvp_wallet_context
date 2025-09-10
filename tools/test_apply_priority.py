import json
import subprocess

import apply_priority as mod


class Stub:
    def __init__(self):
        self.calls = []
        self.queue = []

    def add(self, matcher, result):
        """matcher(cmd:list)->bool; result is CompletedProcess"""
        self.queue.append((matcher, result))

    def __call__(self, cmd, check=True, text=True, capture_output=True):  # noqa: ARG002
        self.calls.append(list(cmd))
        for matcher, result in self.queue:
            try:
                if matcher(cmd):
                    return result
            except Exception:
                continue
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")


def cp_ok(cmd, out="", err=""):
    return subprocess.CompletedProcess(cmd, 0, stdout=out, stderr=err)


def cp_fail(cmd, out="", err=""):
    return subprocess.CompletedProcess(cmd, 1, stdout=out, stderr=err)


def test_gh_exists(monkeypatch):
    stub = Stub()
    stub.add(
        lambda c: c == ["gh", "--version"], cp_ok(["gh", "--version"], out="gh version")
    )
    monkeypatch.setattr(mod, "run", stub)
    assert mod.gh_exists() is True


def test_ensure_label_paths(monkeypatch, capsys):
    # Existing label: view rc=0
    stub = Stub()
    stub.add(
        lambda c: c[0] == "gh" and "label" in c and "view" in c,
        cp_ok(["gh", "label", "view", "P0"]),
    )
    monkeypatch.setattr(mod, "run", stub)
    mod.ensure_label("P0", "ededed", "Priority P0", repo=None)
    assert any(
        (call[0] == "gh" and "label" in call and "view" in call) for call in stub.calls
    )

    # Creation fails -> warning to stderr
    stub = Stub()
    # view fails
    stub.add(
        lambda c: c[:3] == ["gh", "label", "view"],
        cp_fail(["gh", "label", "view", "P9"]),
    )
    # create fails
    stub.add(
        lambda c: c[:3] == ["gh", "label", "create"],
        cp_fail(["gh", "label", "create", "P9"], err="denied"),
    )
    monkeypatch.setattr(mod, "run", stub)
    mod.ensure_label("P9", "cccccc", "Priority P9", repo=None)
    err = capsys.readouterr().err
    assert "failed to create label 'P9'" in err


def test_ensure_milestone_existing(monkeypatch):
    # List milestones returns two lines of JSON
    listing = (
        json.dumps({"title": "M1", "number": 1})
        + "\n"
        + json.dumps({"title": "M2", "number": 2})
        + "\n"
    )
    calls = []

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        calls.append(list(cmd))
        if cmd[:2] == ["gh", "api"] and "-q" in cmd:
            return cp_ok(cmd, out=listing)
        raise AssertionError(f"unexpected run: {cmd}")

    monkeypatch.setattr(mod, "run", fake_run)
    assert mod.ensure_milestone("M2", "desc", "o/r") == 2
    # Only listing call should have occurred
    assert len(calls) == 1


def test_ensure_milestone_create(monkeypatch):
    # First list empty, then POST returns created milestone
    created = {"number": 42}
    calls = []

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        calls.append(list(cmd))
        if cmd[:2] == ["gh", "api"] and "-q" in cmd:
            return cp_ok(cmd, out="")
        if cmd[:2] == ["gh", "api"] and "-X" in cmd and "POST" in cmd:
            return cp_ok(cmd, out=json.dumps(created))
        raise AssertionError(f"unexpected run: {cmd}")

    monkeypatch.setattr(mod, "run", fake_run)
    assert mod.ensure_milestone("M9", "desc", "o/r") == 42
    assert any("POST" in c for c in calls)


def test_get_issue_map(monkeypatch):
    issues = [{"number": 7, "title": "A"}, {"number": 8, "title": "B"}]
    stub = Stub()
    stub.add(
        lambda c: c[:2] == ["gh", "issue"],
        cp_ok(["gh", "issue"], out=json.dumps(issues)),
    )
    monkeypatch.setattr(mod, "run", stub)
    m = mod.get_issue_map("o/r")
    assert m == {"A": 7, "B": 8}


def test_apply_issue_edit_success(monkeypatch):
    stub = Stub()
    # issue edit ok
    stub.add(lambda c: c[:3] == ["gh", "issue", "edit"], cp_ok(["gh", "issue", "edit"]))
    monkeypatch.setattr(mod, "run", stub)
    mod.apply_issue("o/r", 10, "P0", "M1")
    # First call should be issue edit
    assert stub.calls[0][:3] == ["gh", "issue", "edit"]


def test_apply_issue_fallback(monkeypatch):
    calls = []
    lines = json.dumps({"title": "M1", "number": 1}) + "\n"
    issue_obj = {"labels": [{"name": "existing"}]}

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        calls.append(list(cmd))
        # First edit fails
        if cmd[:3] == ["gh", "issue", "edit"]:
            return cp_fail(cmd)
        # List milestones
        if cmd[:2] == ["gh", "api"] and "milestones" in cmd[2] and "-q" in cmd:
            return cp_ok(cmd, out=lines)
        # Issue get
        if cmd[:2] == ["gh", "api"] and "/issues/" in cmd[2] and len(cmd) == 3:
            return cp_ok(cmd, out=json.dumps(issue_obj))
        # Patch
        if cmd[:2] == ["gh", "api"] and "/issues/" in cmd[2] and "PATCH" in cmd:
            return cp_ok(cmd)
        raise AssertionError(f"unexpected run: {cmd}")

    monkeypatch.setattr(mod, "run", fake_run)
    mod.apply_issue("o/r", 99, "P0", "M1")
    assert any("/issues/" in c[2] and "-X" in c and "PATCH" in c for c in calls)


def test_main_plan_flow(monkeypatch, capsys):
    # Exercise main() to cover plan building and iteration
    calls = {
        "labels": [],
        "milestones": [],
        "apply": [],
    }

    monkeypatch.setattr(mod, "gh_exists", lambda: True)

    def fake_ensure_label(
        name, color, description, repo=None, dry_run=False
    ):  # noqa: ARG001
        calls["labels"].append((name, color))

    def fake_ensure_milestone(title, description, repo):  # noqa: ARG001
        calls["milestones"].append(title)
        # Return a dummy milestone number
        return len(calls["milestones"]) + 100

    # Only a subset of titles exists in repo mapping
    mapping = {
        "Scaffold repo structure (Go-first layout)": 1,
        "Epic: API": 2,
    }

    def fake_get_issue_map(repo):  # noqa: ARG001
        return mapping

    def fake_apply_issue(repo, number, prio, ms_title):  # noqa: ARG001
        calls["apply"].append((number, prio, ms_title))

    monkeypatch.setattr(mod, "ensure_label", fake_ensure_label)
    monkeypatch.setattr(mod, "ensure_milestone", fake_ensure_milestone)
    monkeypatch.setattr(mod, "get_issue_map", fake_get_issue_map)
    monkeypatch.setattr(mod, "apply_issue", fake_apply_issue)

    # Run main
    import sys as _sys

    _sys.argv = ["apply_priority.py", "--repo", "o/r"]
    mod.main()

    # We applied at least the two mapped issues
    assert any(n == 1 for (n, _, _) in calls["apply"])  # Scaffold repo structure
    assert any(n == 2 for (n, _, _) in calls["apply"])  # Epic: API
    # Priority labels ensured
    ensured = {lbl for (lbl, _) in calls["labels"]}
    assert {"P0", "P1", "P2"}.issubset(ensured)
    # Milestones ensured (some subset)
    assert any(t.startswith("M0 ") for t in calls["milestones"])
