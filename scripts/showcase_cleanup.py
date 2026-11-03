#!/usr/bin/env python3
"""Move earlier provider showcase pages to the Notion trash.

The showcase workflow builds a fresh "Provider showcase" page on every run
and keeps no Terraform state between runs. This script runs first and
trashes the previous copies. It only touches child pages of the parent page
whose title starts with the prefix, never anything else. Trashed pages stay
recoverable in Notion for 30 days.

Standard library only, so it runs anywhere Python 3.9+ does.
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API_BASE = "https://api.notion.com/v1"
NOTION_VERSION = "2026-03-11"
DEFAULT_PREFIX = "Provider showcase"
MAX_ATTEMPTS = 6
# Notion's edge sometimes answers with a 5xx error page that succeeds on a
# second try. Those are retried along with 429s. Anything else fails.
RETRY_STATUSES = {429, 500, 502, 503, 504}


class NotionError(Exception):
    pass


def request(token, method, path, body=None):
    url = API_BASE + path
    data = None if body is None else json.dumps(body).encode("utf-8")
    headers = {
        "Authorization": "Bearer " + token,
        "Notion-Version": NOTION_VERSION,
        "Content-Type": "application/json",
    }
    for attempt in range(1, MAX_ATTEMPTS + 1):
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as err:
            detail = err.read().decode("utf-8", errors="replace")[:500]
            if err.code in RETRY_STATUSES and attempt < MAX_ATTEMPTS:
                wait = retry_after(err, attempt)
                print(f"{method} {path} returned {err.code}, retrying in {wait:.0f}s", file=sys.stderr)
                time.sleep(wait)
                continue
            raise NotionError(f"{method} {path} failed with HTTP {err.code}: {detail}") from None
        except urllib.error.URLError as err:
            if attempt < MAX_ATTEMPTS:
                wait = min(2 ** attempt, 30)
                print(f"{method} {path} failed ({err.reason}), retrying in {wait}s", file=sys.stderr)
                time.sleep(wait)
                continue
            raise NotionError(f"{method} {path} failed: {err.reason}") from None
    raise NotionError(f"{method} {path} failed after {MAX_ATTEMPTS} attempts")


def retry_after(err, attempt):
    header = err.headers.get("Retry-After") if err.headers else None
    if header:
        try:
            return max(float(header), 1.0)
        except ValueError:
            pass
    return float(min(2 ** attempt, 30))


def child_pages(token, parent_id):
    """Yield (id, title) for every child page block of the parent."""
    cursor = None
    while True:
        query = {"page_size": "100"}
        if cursor:
            query["start_cursor"] = cursor
        path = f"/blocks/{parent_id}/children?" + urllib.parse.urlencode(query)
        data = request(token, "GET", path)
        for block in data.get("results", []):
            if block.get("type") == "child_page":
                yield block["id"], block.get("child_page", {}).get("title", "")
        if not data.get("has_more"):
            return
        cursor = data.get("next_cursor")
        if not cursor:
            return


def main():
    parser = argparse.ArgumentParser(
        description="Move child pages whose title starts with a prefix (default "
        f'"{DEFAULT_PREFIX}") to the Notion trash. Reads the token from NOTION_TOKEN.'
    )
    parser.add_argument(
        "--parent-page-id",
        default=os.environ.get("TF_VAR_parent_page_id") or os.environ.get("NOTION_TEST_PARENT_PAGE_ID"),
        help="Page whose children are checked. Defaults to TF_VAR_parent_page_id, then NOTION_TEST_PARENT_PAGE_ID.",
    )
    parser.add_argument(
        "--prefix",
        default=DEFAULT_PREFIX,
        help=f'Title prefix to match (default "{DEFAULT_PREFIX}").',
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List the pages that would be trashed without changing anything.",
    )
    args = parser.parse_args()

    token = os.environ.get("NOTION_TOKEN", "")
    if not token:
        parser.error("NOTION_TOKEN is not set")
    parent = (args.parent_page_id or "").strip()
    if not parent:
        parser.error("no parent page ID. Pass --parent-page-id or set TF_VAR_parent_page_id")
    if not args.prefix.strip():
        parser.error("--prefix can't be empty, it would match every child page")

    try:
        matches = [(pid, title) for pid, title in child_pages(token, parent) if title.startswith(args.prefix)]
        if not matches:
            print(f'No child pages starting with "{args.prefix}" found.')
            return 0
        for pid, title in matches:
            if args.dry_run:
                print(f"Would trash: {title} ({pid})")
                continue
            request(token, "PATCH", f"/pages/{pid}", {"in_trash": True})
            print(f"Trashed: {title} ({pid})")
    except NotionError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1

    verb = "would be trashed" if args.dry_run else "trashed"
    print(f"{len(matches)} page(s) {verb}. Trashed pages stay recoverable in Notion for 30 days.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
