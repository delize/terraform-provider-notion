#!/usr/bin/env python3
"""Create or update the repository rulesets defined in this folder.

Each *.json file here is one ruleset. A ruleset on GitHub with the same
name is updated in place, otherwise a new one is created. Uses the gh CLI,
so run `gh auth login` first. Needs admin on the repository.

    python3 .github/rulesets/apply.py            # apply
    python3 .github/rulesets/apply.py --dry-run  # show what would change
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent


def gh_api(*args, payload=None):
    cmd = ["gh", "api", *args]
    if payload is not None:
        cmd += ["--input", "-"]
    result = subprocess.run(
        cmd,
        input=json.dumps(payload) if payload is not None else None,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        sys.exit(f"gh api {' '.join(args)} failed:\n{result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else None


def current_repo():
    out = subprocess.run(
        ["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
        capture_output=True,
        text=True,
        check=True,
    )
    return out.stdout.strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", help="owner/name, defaults to the current repo")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    repo = args.repo or current_repo()
    existing = {r["name"]: r["id"] for r in gh_api(f"repos/{repo}/rulesets") or []}

    for path in sorted(HERE.glob("*.json")):
        ruleset = json.loads(path.read_text())
        name = ruleset["name"]
        if name in existing:
            action, method, url = "update", "PUT", f"repos/{repo}/rulesets/{existing[name]}"
        else:
            action, method, url = "create", "POST", f"repos/{repo}/rulesets"
        print(f"{action}: {name} ({path.name}) on {repo}")
        if args.dry_run:
            continue
        result = gh_api("-X", method, url, payload=ruleset)
        print(f"  ok, ruleset id {result['id']}")


if __name__ == "__main__":
    main()
