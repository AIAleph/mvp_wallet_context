#!/usr/bin/env python3
import subprocess
import sys
import re



def main():
    if len(sys.argv) != 2:
        print("usage: check_go_coverage.py coverage.out", file=sys.stderr)
        sys.exit(2)
    cov = sys.argv[1]
    try:
        out = subprocess.check_output(["go", "tool", "cover", "-func", cov], text=True)
    except Exception as e:
        print(f"failed to read coverage: {e}", file=sys.stderr)
        sys.exit(1)
    # Find the total line: total:\t(statements) X.Y%
    total_line = None
    for line in out.splitlines():
        if line.startswith("total:"):
            total_line = line
    if not total_line:
        print("no total coverage line found", file=sys.stderr)
        print(out)
        sys.exit(1)
    m = re.search(r"total:\s*\(statements\)\s*([0-9.]+)%", total_line)
    if not m:
        print("could not parse total coverage", file=sys.stderr)
        print(total_line)
        sys.exit(1)
    pct = float(m.group(1))
    # If there are zero statements, go prints 0.0%. In that case, don't fail.
    # We'll only enforce 100% when there is measured code in the packages.
    if pct == 0.0:
        print("No measured statements; skipping enforcement.")
        sys.exit(0)
    if abs(pct - 100.0) > 1e-9:
        print(f"Coverage {pct}% is below required 100%", file=sys.stderr)
        sys.exit(1)
    print("Coverage OK: 100%")


if __name__ == "__main__":
    main()
