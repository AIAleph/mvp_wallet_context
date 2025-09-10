import json
import subprocess
import sys
from pathlib import Path

import create_github_issues as mod
import pytest


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


def test_run_wrapper(monkeypatch):
    called = {}

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        called["ok"] = True
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")

    monkeypatch.setattr(subprocess, "run", fake_run)
    res = mod.run(["echo", "ok"])  # should call our fake_run
    assert isinstance(res, subprocess.CompletedProcess)
    assert called.get("ok") is True


def test_create_issue_exec(monkeypatch):
    # Ensure non-dry-run path calls run
    captured = {}

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        captured["cmd"] = cmd
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")

    monkeypatch.setattr(mod, "run", fake_run)
    mod.create_issue("T", "B", ["l1"], repo=None, dry_run=False)
    assert captured["cmd"][0] == "gh"


def test_ensure_label_exec(monkeypatch):
    # View fails then create has no description branch
    seq = []

    def fake_run(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        if len(seq) == 0 and cmd[:3] == ["gh", "label", "view"]:
            seq.append("view")
            return subprocess.CompletedProcess(cmd, 1, stdout="", stderr="")
        if len(seq) == 1 and cmd[:3] == ["gh", "label", "create"]:
            seq.append("create")
            return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
        raise AssertionError(f"unexpected run: {cmd}")

    monkeypatch.setattr(mod, "run", fake_run)
    mod.ensure_label("x", color="ededed", description="", repo=None, dry_run=False)
    assert seq == ["view", "create"]


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


def test_main_gh_missing(monkeypatch):
    monkeypatch.setattr(mod, "gh_exists", lambda: False)
    import sys as _sys

    _sys.argv = ["create_github_issues.py", "--repo", "o/r"]
    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 1


def test_main_ensure_and_create_errors(monkeypatch, capsys):
    # Make gh_exists return True
    monkeypatch.setattr(mod, "gh_exists", lambda: True)

    # cause ensure_label to raise on first item to hit warning path, but continue
    calls = {"ensured": 0, "created": 0}

    def fake_ensure_label(name, color, desc, repo=None, dry_run=False):  # noqa: ARG001
        calls["ensured"] += 1
        if name == "backend":
            raise subprocess.CalledProcessError(1, ["gh"])  # warning path

    def fake_create_issue(
        title, body, labels, repo=None, dry_run=False
    ):  # noqa: ARG001
        calls["created"] += 1
        if title == "T2":
            raise subprocess.CalledProcessError(1, ["gh"])  # error path

    monkeypatch.setattr(mod, "ensure_label", fake_ensure_label)
    monkeypatch.setattr(mod, "create_issue", fake_create_issue)

    sample = [
        {"title": "T1", "labels": ["tooling"], "body": "B1"},
        {"title": "", "labels": [], "body": "B2"},  # skipped
        {"title": "T2", "labels": [], "body": "B3"},  # error
    ]
    # Read issues.json content from our sample
    monkeypatch.setattr(Path, "read_text", lambda self=None: json.dumps(sample))  # type: ignore[misc]

    import sys as _sys

    _sys.argv = ["create_github_issues.py", "--repo", "o/r"]
    mod.main()
    out = capsys.readouterr().out
    assert "Skipping issue without title" in out
    assert "Created issue: T1" in out
    assert "Error creating issue 'T2'" in out


def test_run_module_main_safe(monkeypatch):
    # Force gh_exists() to be false in the spawned module by stubbing subprocess.run
    def fake_subproc(cmd, check=True, text=True, capture_output=True):  # noqa: ARG001
        raise RuntimeError("no gh")

    import subprocess as _sp
    import runpy as _runpy
    import sys as _sys

    monkeypatch.setattr(_sp, "run", fake_subproc)
    _sys.argv = ["create_github_issues.py"]
    with pytest.raises(SystemExit):
        _runpy.run_module("create_github_issues", run_name="__main__")
