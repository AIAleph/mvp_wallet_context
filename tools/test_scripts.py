import os
import shutil
import stat
import subprocess
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]


def run(cmd, env=None, cwd=None):
    target_cwd = cwd if cwd is not None else REPO_ROOT
    return subprocess.run(
        cmd,
        shell=True,
        env=env,
        cwd=target_cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


def test_shell_scripts_syntax():
    # bash -n checks syntax only
    r = run("bash -n scripts/*.sh")
    assert r.returncode == 0, r.stderr.decode()


def _make_exe(dir: Path, name: str, content: str) -> Path:
    p = dir / name
    p.write_text(content)
    p.chmod(p.stat().st_mode | stat.S_IXUSR)
    return p


def test_schema_sh_validates_and_applies_with_stubbed_client(tmp_path: Path):
    # Create a dummy schema file
    schema = tmp_path / "test.sql"
    schema.write_text("SELECT 1;\n")
    # Stub clickhouse-client that accepts create DB and queries-file
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    _make_exe(
        bin_dir,
        "clickhouse-client",
        """#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "-q" ]]; then exit 0; fi
if [[ "$1" == "--database" ]]; then shift; fi
exit 0
""",
    )
    env = os.environ.copy()
    env.pop("CLICKHOUSE_URL", None)
    env.pop("CLICKHOUSE_DSN", None)
    env.pop("CLICKHOUSE_DB", None)
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    env["CH_DB"] = "testdb"
    env["SCHEMA_FILE"] = str(schema)
    r = run("scripts/schema.sh", env=env)
    assert r.returncode == 0, r.stderr.decode()


def test_api_sh_npm_fallback_and_commands(tmp_path: Path):
    # Stub npm to fail for 'ci' and succeed for others; capture calls
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    npm_log = tmp_path / "npm.log"
    _make_exe(
        bin_dir,
        "npm",
        f"""#!/usr/bin/env bash
echo "$@" >> "{npm_log}"
if [[ "$1" == "ci" ]]; then exit 1; fi
exit 0
""",
    )
    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    # Remove node_modules to trigger install path
    api_dir = Path.cwd() / "api"
    nm = api_dir / "node_modules"
    if nm.exists():
        # ensure empty to enforce install path
        for child in nm.iterdir():
            if child.is_dir():
                shutil.rmtree(child)
            else:
                child.unlink()
    # Run test subcommand; stub npm will handle it
    r = run("scripts/api.sh test", env=env)
    assert r.returncode == 0, r.stderr.decode()
    log = npm_log.read_text()
    # Either npm ci (then fallback) or direct npm run test if node_modules exist
    assert ("ci" in log) or ("run test" in log)


def test_ingest_sh_runs_with_stubbed_go(tmp_path: Path):
    # Stub go to always succeed
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    _make_exe(
        bin_dir,
        "go",
        """#!/usr/bin/env bash
exit 0
""",
    )
    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    addr = "0x" + "a" * 40
    r = run(
        f"scripts/ingest.sh ADDRESS={addr} MODE=backfill FROM=0 TO=0 BATCH=10 SCHEMA=dev",
        env=env,
    )
    assert r.returncode == 0, r.stderr.decode()


def test_migrate_schema_records_version(tmp_path: Path):
    # Stub clickhouse-client to capture -q SQL
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    log = tmp_path / "sql.log"
    _make_exe(
        bin_dir,
        "clickhouse-client",
        f"""#!/usr/bin/env bash
set -euo pipefail
while (($#)); do
  case "$1" in
    --database)
      shift 2
      ;;
    --param_*)
      shift
      ;;
    -q|--query|-n)
      shift
      if (($#)); then
        echo "$1" >> "{log}"
      fi
      exit 0
      ;;
    *)
      shift
      ;;
  esac
done
exit 0
""",
    )
    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    env["CLICKHOUSE_DB"] = "wallets"
    r = run("bash scripts/migrate_schema.sh TO=dev", env=env)
    assert r.returncode == 0, r.stderr.decode()
    content = log.read_text()
    assert "INSERT INTO schema_version" in content
