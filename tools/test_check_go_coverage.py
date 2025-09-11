import subprocess
import sys

import pytest

import check_go_coverage as mod
import runpy


def _set_argv(tmp_path, cov_name="coverage.out"):
    cov = tmp_path / cov_name
    cov.write_text("dummy")
    sys.argv = ["check_go_coverage.py", str(cov)]


def test_coverage_ok(monkeypatch, tmp_path, capsys):
    _set_argv(tmp_path)

    def fake_check_output(cmd, text=False):  # noqa: ARG001
        return (
            "pkg/foo.go:12: func A 100.0%\n"
            "pkg/bar.go:34: func B 100.0%\n"
            "total:\t(statements) 100.0%\n"
        )

    monkeypatch.setattr(subprocess, "check_output", fake_check_output)

    # Expect a clean exit
    mod.main()
    out = capsys.readouterr().out
    assert "Coverage OK: 100%" in out


def test_coverage_below(monkeypatch, tmp_path, capsys):
    _set_argv(tmp_path)

    def fake_check_output(cmd, text=False):  # noqa: ARG001
        return (
            "pkg/foo.go:12: func A 100.0%\n"
            "pkg/bar.go:34: func B 98.5%\n"
            "total:\t(statements) 98.5%\n"
        )

    monkeypatch.setattr(subprocess, "check_output", fake_check_output)

    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 1
    err = capsys.readouterr().err
    assert "below required 100%" in err


def test_no_statements_skip(monkeypatch, tmp_path, capsys):
    _set_argv(tmp_path)

    def fake_check_output(cmd, text=False):  # noqa: ARG001
        return "pkg/foo.go:12: func A 0.0%\n" "total:\t(statements) 0.0%\n"

    monkeypatch.setattr(subprocess, "check_output", fake_check_output)

    # Exits with code 0 and prints skip message
    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 0
    out = capsys.readouterr().out
    assert "No measured statements; skipping enforcement." in out


def test_missing_total_line(monkeypatch, tmp_path, capsys):
    _set_argv(tmp_path)

    def fake_check_output(cmd, text=False):  # noqa: ARG001
        return "pkg/foo.go:12: func A 100.0%\n"

    monkeypatch.setattr(subprocess, "check_output", fake_check_output)

    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 1
    err = capsys.readouterr().err
    assert "no total coverage line found" in err


def test_usage_wrong_args(capsys):
    sys.argv = ["check_go_coverage.py"]
    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 2
    err = capsys.readouterr().err
    assert "usage:" in err.lower()


def test_failed_read(monkeypatch, tmp_path, capsys):
    cov = tmp_path / "coverage.out"
    cov.write_text("")
    sys.argv = ["check_go_coverage.py", str(cov)]

    def boom(cmd, text=False):  # noqa: ARG001
        raise RuntimeError("nope")

    monkeypatch.setattr(subprocess, "check_output", boom)
    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 1
    err = capsys.readouterr().err
    assert "failed to read coverage" in err


def test_parse_fail(monkeypatch, tmp_path, capsys):
    _set_argv(tmp_path)

    def weird(cmd, text=False):  # noqa: ARG001
        return "total: strange format 100 percent\n"

    monkeypatch.setattr(subprocess, "check_output", weird)
    with pytest.raises(SystemExit) as ei:
        mod.main()
    assert ei.value.code == 1
    err = capsys.readouterr().err
    assert "could not parse total coverage" in err


def test_run_as_main_uses_entrypoint(tmp_path):
    cov = tmp_path / "coverage.out"
    cov.write_text("")
    sys.argv = ["check_go_coverage.py"]
    with pytest.raises(SystemExit):
        runpy.run_module("check_go_coverage", run_name="__main__")
