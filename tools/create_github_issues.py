#!/usr/bin/env python3
"""
Create GitHub issues and labels from tools/issues.json using the GitHub CLI (gh).

Prereqs:
- Install GitHub CLI: https://cli.github.com/
- Authenticate: gh auth login
- Provide target repo with --repo OWNER/NAME (recommended). If omitted, gh will use the current repo remote.

Usage:
  python tools/create_github_issues.py --repo OWNER/NAME [--dry-run]

Notes:
- Idempotency is best-effort: labels are created if missing; issues are created without dedup checks (avoid re-running or use --dry-run).
"""
import argparse
import json
import subprocess
import sys
from pathlib import Path
from typing import Optional


def run(cmd, check=True):
    return subprocess.run(cmd, check=check, text=True, capture_output=True)


def gh_exists():
    try:
        run(["gh", "--version"])  # type: ignore[arg-type]
        return True
    except Exception:
        return False


def ensure_label(name: str, color: str = "ededed", description: str = "", repo: Optional[str] = None, dry_run: bool = False):
    # Check if label exists
    base = ["gh"]
    if repo:
        base += ["--repo", repo]
    res = run(base + ["label", "view", name], check=False)
    if res.returncode == 0:
        return
    # Create label
    cmd = base + ["label", "create", name, "--color", color]
    if description:
        cmd += ["--description", description]
    if dry_run:
        print("DRY-RUN:", " ".join(cmd))
        return
    run(cmd, check=True)


def create_issue(title: str, body: str, labels: list[str], repo: Optional[str] = None, dry_run: bool = False):
    cmd = ["gh"]
    if repo:
        cmd += ["--repo", repo]
    cmd += ["issue", "create", "--title", title, "--body", body]
    # Multiple --label flags are supported
    for lbl in labels:
        cmd += ["--label", lbl]
    if dry_run:
        print("DRY-RUN:", " ".join(cmd))
        return
    run(cmd, check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", help="Target GitHub repo OWNER/NAME", default=None)
    parser.add_argument("--dry-run", action="store_true", help="Print gh commands without executing")
    args = parser.parse_args()
    dry_run = args.dry_run
    repo = args.repo
    if not gh_exists():
        print("Error: gh CLI not found. Install from https://cli.github.com and run 'gh auth login'.", file=sys.stderr)
        sys.exit(1)

    issues_path = Path("tools/issues.json")
    data = json.loads(issues_path.read_text())

    # Ensure commonly used labels exist with colors
    label_palette = {
        "epic": ("5319e7", "High-level epic"),
        "backend": ("1d76db", "Go services and ingestion"),
        "ingestion": ("0052cc", "Fetching and cursors"),
        "normalization": ("0e8a16", "Decoders and mapping"),
        "enrichment": ("c2e0c6", "EOA/contract, metadata, labels"),
        "sql": ("b60205", "ClickHouse schema and queries"),
        "api": ("fbca04", "TypeScript API"),
        "embeddings": ("5319e7", "Vector search and models"),
        "ops": ("d93f0b", "Reliability and observability"),
        "tooling": ("c5def5", "Dev tools and CI"),
        "testing": ("0e8a16", "Fixtures and tests"),
        "docs": ("0366d6", "Documentation"),
    }

    for name, (color, desc) in label_palette.items():
        try:
            ensure_label(name, color, desc, repo=repo, dry_run=dry_run)
            if dry_run:
                print(f"Ensured label '{name}'")
        except subprocess.CalledProcessError as e:
            print(f"Warning: failed ensuring label '{name}': {e.stderr}")

    # Create issues
    for item in data:
        title = item.get("title", "").strip()
        body = item.get("body", "").strip()
        labels = item.get("labels", [])
        if not title:
            print("Skipping issue without title")
            continue
        try:
            create_issue(title, body, labels, repo=repo, dry_run=dry_run)
            print(f"Created issue: {title}")
        except subprocess.CalledProcessError as e:
            print(f"Error creating issue '{title}': {e.stderr}")


if __name__ == "__main__":
    main()
