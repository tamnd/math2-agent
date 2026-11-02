#!/usr/bin/env python3
"""Upload missing content-addressed social image packs to Cloudflare R2.

The site build writes a small set of .pack files outside the Pages directory.
Run this before pages deploy so every image URL in the new HTML is readable.
"""

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import hashlib
import json
import os
from pathlib import Path
import time
from urllib.parse import urlencode
from urllib.request import Request, urlopen


def api(url, token, *, data=None, method="GET"):
    headers = {"Authorization": f"Bearer {token}"}
    if data is not None:
        headers["Content-Type"] = "application/octet-stream"
    req = Request(url, data=data, headers=headers, method=method)
    with urlopen(req, timeout=120) as response:
        result = json.load(response)
    if not result.get("success"):
        raise RuntimeError(f"Cloudflare R2 request failed: {result.get('errors')}")
    return result


def existing_objects(base, token):
    objects = {}
    cursor = None
    while True:
        query = {"prefix": "social/", "per_page": 1000}
        if cursor:
            query["cursor"] = cursor
        result = api(base + "?" + urlencode(query), token)
        for obj in result["result"]:
            objects[obj["key"]] = int(obj["size"])
        info = result.get("result_info") or {}
        if not info.get("is_truncated"):
            return objects
        cursor = info["cursor"]


def upload_one(base, token, path):
    data = path.read_bytes()
    digest = hashlib.sha256(data).hexdigest()[:24]
    if path.name != digest + ".pack":
        raise ValueError(f"pack name does not match its contents: {path.name}")
    key = "social/" + path.name
    for attempt in range(4):
        try:
            result = api(base + "/" + key, token, data=data, method="PUT")
            if int(result["result"]["size"]) != len(data):
                raise RuntimeError(f"uploaded size differs for {key}")
            return key, len(data)
        except Exception:
            if attempt == 3:
                raise
            time.sleep(2**attempt)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--bucket", default="math2-social")
    args = parser.parse_args()
    token = os.environ["CLOUDFLARE_API_TOKEN"]
    account = os.environ["CLOUDFLARE_ACCOUNT_ID"]
    base = f"https://api.cloudflare.com/client/v4/accounts/{account}/r2/buckets/{args.bucket}/objects"
    packs = sorted(args.directory.glob("*.pack"))
    if not packs:
        raise SystemExit("no social image packs to upload")
    old = existing_objects(base, token)
    missing = []
    for path in packs:
        key = "social/" + path.name
        if key in old:
            if old[key] != path.stat().st_size:
                raise SystemExit(f"existing pack has the wrong size: {key}")
        else:
            missing.append(path)
    print(f"{len(packs)} social packs, {len(missing)} new")
    with ThreadPoolExecutor(max_workers=8) as pool:
        futures = [pool.submit(upload_one, base, token, path) for path in missing]
        for i, future in enumerate(as_completed(futures), 1):
            future.result()
            if i % 16 == 0 or i == len(missing):
                print(f"uploaded {i}/{len(missing)} packs", flush=True)
    current = existing_objects(base, token)
    if any(current.get("social/" + path.name) != path.stat().st_size for path in packs):
        raise SystemExit("R2 does not yet list every social pack")
    print("all social packs verified in R2")


if __name__ == "__main__":
    main()
