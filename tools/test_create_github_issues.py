import json
import subprocess
import sys
from pathlib import Path

import create_github_issues as mod


class StubRun:
    def __init__(self, mapping=None):
        self.calls = []
        self.mapping = mapping or {}

    def __call__(self, cmd, check=True, text=True, capture_output=True):  # noqa: ARG002
        self.calls.append(list(cmd))
        key = tuple(cmd)
        ret = self.mapping.get(key)
        if ret is not None:
            return ret
        # Default: succeed
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")


def test_gh_exists(monkeypatch):
    # Success path
    stub = StubRun(
        {
            ("gh", "--version"): subprocess.CompletedProcess(
                ["gh", "--version"], 0, stdout="gh version", stderr=""
            )
        }
    )
    monkeypatch.setattr(mod, "run", stub)
    assert mod.gh_exists() is True

    # Failure path
    def boom(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        raise RuntimeError("oops")

    monkeypatch.setattr(mod, "run", boom)
    assert mod.gh_exists() is False


def test_ensure_label_exists(monkeypatch):
    # label view returns 0 -> no creation
    view_ok = subprocess.CompletedProcess(
        ["gh", "label", "view", "name"], 0, stdout="", stderr=""
    )
    stub = StubRun(
        {
            ("gh", "label", "view", "name"): view_ok,
        }
    )
    monkeypatch.setattr(mod, "run", stub)
    mod.ensure_label(
        "name", color="ededed", description="desc", repo=None, dry_run=False
    )
    # Only one call (view)
    assert len(stub.calls) == 1
    assert stub.calls[0][:3] == ["gh", "label", "view"]


def test_ensure_label_create_dry_run(monkeypatch, capsys):
    # label view returns non-zero -> would create; dry-run prints command
    view_fail = subprocess.CompletedProcess(
        ["gh", "label", "view", "name"], 1, stdout="", stderr=""
    )
    stub = StubRun(
        {
            ("gh", "label", "view", "name"): view_fail,
        }
    )
    monkeypatch.setattr(mod, "run", stub)
    mod.ensure_label(
        "name", color="ededed", description="desc", repo=None, dry_run=True
    )
    out = capsys.readouterr().out
    assert "DRY-RUN:" in out
    assert "label create name --color ededed" in out


def test_create_issue_dry_run(monkeypatch, capsys):
    stub = StubRun()
    monkeypatch.setattr(mod, "run", stub)
    mod.create_issue("t", "b", ["l1", "l2"], repo="r/x", dry_run=True)
    out = capsys.readouterr().out
    assert "DRY-RUN:" in out
    assert "issue create --title t --body b" in out


def test_main_dry_run_minimal(monkeypatch, capsys, tmp_path):
    # Make gh_exists return True
    monkeypatch.setattr(mod, "gh_exists", lambda: True)

    # Stub run: label view returns ok (so no creation); no other calls needed
    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        if cmd[:3] == ["gh", "label", "view"]:
            return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
        # For issue create (dry-run), not actually executed
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")

    monkeypatch.setattr(mod, "run", fake_run)

    # Provide a tiny issues.json via monkeypatching Path.read_text
    sample = [
        {"title": "T1", "labels": ["tooling"], "body": "B1"},
        {"title": "T2", "labels": [], "body": "B2"},
    ]
    monkeypatch.setattr(Path, "read_text", lambda self=None: json.dumps(sample))  # type: ignore[misc]

    # sys.argv for --dry-run
    sys.argv = ["create_github_issues.py", "--repo", "o/r", "--dry-run"]
    mod.main()
    out = capsys.readouterr().out
    assert "DRY-RUN:" in out
    assert "issue create --title T1" in out
    assert "issue create --title T2" in out
