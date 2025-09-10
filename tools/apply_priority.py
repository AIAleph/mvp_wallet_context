#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from typing import Dict, Tuple, Optional



def run(cmd, check=True):
    return subprocess.run(cmd, check=check, text=True, capture_output=True)



def gh_exists():
    try:
        run(["gh", "--version"])  # type: ignore[arg-type]
        return True
    except Exception:
        return False



def ensure_label(
    name: str, color: str, description: str, repo: Optional[str] = None
):
    base = ["gh"]
    if repo:
        base += ["--repo", repo]
    # Check
    res = run(base + ["label", "view", name], check=False)
    if res.returncode == 0:
        return
    # Create
    res = run(
        base
        + [
            "label",
            "create",
            name,
            "--color",
            color,
            "--description",
            description,
        ],
        check=False,
    )
    if res.returncode != 0:
        # Non-fatal: might lack permission; adding labels to issues may still work or fail gracefully.
        sys.stderr.write(f"Warning: failed to create label '{name}': {res.stderr}\n")



def ensure_milestone(title: str, description: str, repo: str) -> int:
    # List milestones
    res = run(
        [
            "gh",
            "api",
            f"repos/{repo}/milestones",
            "-q",
            ".[] | {title: .title, number: .number}",
        ]
    )
    numbers = {}
    # Parse line-delimited JSON objects
    for line in res.stdout.strip().splitlines():
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
            numbers[obj["title"]] = obj["number"]
        except Exception:
            continue
    if title in numbers:
        return int(numbers[title])
    # Create
    create = run(
        [
            "gh",
            "api",
            f"repos/{repo}/milestones",
            "-X",
            "POST",
            "-f",
            f"title={title}",
            "-f",
            "state=open",
            "-f",
            f"description={description}",
        ]
    )
    try:
        created = json.loads(create.stdout)
        return int(created["number"])
    except Exception as e:
        print("Failed to create milestone:", e, file=sys.stderr)
        print(create.stdout, create.stderr, file=sys.stderr)
        raise



def get_issue_map(repo: str) -> Dict[str, int]:
    res = run(
        [
            "gh",
            "issue",
            "list",
            "--repo",
            repo,
            "--limit",
            "200",
            "--state",
            "open",
            "--json",
            "number,title",
        ]
    )
    data = json.loads(res.stdout)
    return {item["title"]: item["number"] for item in data}



def apply_issue(repo: str, number: int, priority: str, milestone_title: str):
    # First try gh issue edit
    cmd = [
        "gh",
        "issue",
        "edit",
        str(number),
        "--repo",
        repo,
        "--add-label",
        priority,
        "--milestone",
        milestone_title,
    ]
    res = run(cmd, check=False)
    if res.returncode == 0:
        return
    # Fallback via API requires milestone number
    # Fetch milestone list
    ms_res = run(
        [
            "gh",
            "api",
            f"repos/{repo}/milestones",
            "-q",
            ".[] | {title: .title, number: .number}",
        ]
    )
    milestones = {}
    for line in ms_res.stdout.strip().splitlines():
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
            milestones[obj["title"]] = obj["number"]
        except Exception:
            continue
    ms_number = milestones.get(milestone_title)
    if not ms_number:
        raise RuntimeError(f"Milestone not found: {milestone_title}")
    # Get existing labels
    issue_get = run(["gh", "api", f"repos/{repo}/issues/{number}"])
    issue = json.loads(issue_get.stdout)
    labels = [lbl["name"] for lbl in issue.get("labels", [])]
    if priority not in labels:
        labels.append(priority)
    # Patch issue
    patch_cmd = [
        "gh",
        "api",
        f"repos/{repo}/issues/{number}",
        "-X",
        "PATCH",
        "-f",
        f"milestone={ms_number}",
    ]
    for lbl in labels:
        patch_cmd += ["-f", f"labels[]={lbl}"]
    run(patch_cmd)



def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", required=True, help="OWNER/REPO")
    args = parser.parse_args()
    repo = args.repo

    if not gh_exists():
        print("Error: gh CLI not found. Install and run 'gh auth login'.", file=sys.stderr)
        sys.exit(1)

    # Ensure priority labels
    ensure_label("P0", "b60205", "Critical path (MVP)", repo)
    ensure_label("P1", "d93f0b", "MVP polish/perf", repo)
    ensure_label("P2", "fbca04", "Beta/optional", repo)

    # Ensure milestones
    ensure_milestone("M0 Setup & Schema", "Scaffold, config, docker, ClickHouse schema, Makefile", repo)
    ensure_milestone("M1 Ingestion Core", "Provider client, cursors, fetchers, normalization, decoders, idempotency, reorgs", repo)
    ensure_milestone("M2 Enrichment + API", "EOA/contract, ERC-165, labels, summary + lists API, counters", repo)
    ensure_milestone("M3 Semantic Search (beta)", "Embeddings pipeline, ANN queries, /search API", repo)

    # Map titles to (priority, milestone_title)
    P0_M0 = [
        "Scaffold repo structure (Go-first layout)",
        "Define 12-factor configuration",
        "Tooling: docker-compose for ClickHouse (and Redis)",
        "Makefile and scripts",
        "SQL: Author ClickHouse schema.sql",
        "SQL: Add surrogate UIDs and adjust ORDER BY",
    ]
    P0_M1 = [
        "Provider client with retries/backoff and rate limiting",
        "Implement address backfill and delta cursors",
        "Fetcher: external transactions (from/to)",
        "Fetcher: ERC-20/721/1155 transfers via logs",
        "Fetcher: approvals (ERC-20/721/1155)",
        "Fetcher: internal traces",
        "Detect contract creation and persist to contracts",
        "Normalization: unify types and timestamps",
        "Decoders: ERC-20/721/1155 topics & selectors",
        "Idempotency & dedup strategy",
        "Reorg handling with N confirmations",
        "Testing: recorded RPC fixtures and CI",
        "Observability: structured JSON logs",
    ]
    P0_M2 = [
        "Enrichment: EOA vs contract via eth_getCode",
        "Enrichment: ERC-165 and metadata (name/symbol/decimals)",
        "Label registry and confidence scoring",
        "API: POST /v1/address/:address/sync",
        "API: GET /v1/address/:address/summary",
        "API: GET lists (token-transfers, approvals, dapps)",
    ]
    P1_M2 = [
        "SQL: Common queries and projections",
        "Observability: metrics and health checks",
        "Enrichment: proxy detection (EIP-1967/UUPS)",
        "Docs: finalize PRD (EN) and ADR-0001",
        "Docs: ADR-0002 Ingestion idempotency & reorg handling",
    ]
    P2_M3 = [
        "Embeddings job and storage",
        "Semantic search helpers and filters",
        "API: GET /v1/search (text + semantic)",
    ]

    # Epics priority/milestones (optional)
    EPICS = {
        "Epic: Data Model & SQL": ("P0", "M0 Setup & Schema"),
        "Epic: Tooling & CI": ("P0", "M0 Setup & Schema"),
        "Epic: Ingestion Pipeline": ("P0", "M1 Ingestion Core"),
        "Epic: Normalization & Decoders": ("P0", "M1 Ingestion Core"),
        "Epic: Enrichment & Labels": ("P0", "M2 Enrichment + API"),
        "Epic: API": ("P0", "M2 Enrichment + API"),
        "Epic: Observability & Reliability": ("P1", "M2 Enrichment + API"),
        "Epic: Docs": ("P1", "M2 Enrichment + API"),
        "Epic: Embeddings & Search": ("P2", "M3 Semantic Search (beta)"),
    }

    plan: Dict[str, Tuple[str, str]] = {}
    for t in P0_M0:
        plan[t] = ("P0", "M0 Setup & Schema")
    for t in P0_M1:
        plan[t] = ("P0", "M1 Ingestion Core")
    for t in P0_M2:
        plan[t] = ("P0", "M2 Enrichment + API")
    for t in P1_M2:
        plan[t] = ("P1", "M2 Enrichment + API")
    for t in P2_M3:
        plan[t] = ("P2", "M3 Semantic Search (beta)")
    plan.update(EPICS)

    # Build issue map
    title_to_num = get_issue_map(repo)

    missing = []
    for title, (prio, ms_title) in plan.items():
        num = title_to_num.get(title)
        if not num:
            missing.append(title)
            continue
        # Ensure priority label exists (redundant but safe)
        ensure_label(prio, "ededed", f"Priority {prio}", repo)
        # Apply
        try:
            apply_issue(repo, num, prio, ms_title)
            print(f"Updated issue #{num}: {title} -> {prio}, {ms_title}")
        except subprocess.CalledProcessError as e:
            print(f"Failed to update issue #{num}: {title}: {e.stderr}")
    if missing:
        print("Missing issues (titles not found):")
        for t in missing:
            print(" -", t)


if __name__ == "__main__":
    main()
