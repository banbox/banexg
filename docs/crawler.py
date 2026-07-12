#!/usr/bin/env python3
"""Download official documentation for API endpoints used by banexg.

The endpoint inventory is derived from production Go references, so registered
but unused routes do not expand the crawl scope. Generated files are suitable
for review or CI drift checks and contain source hashes and unmatched routes.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import html
import io
import json
import re
import sys
import tarfile
import urllib.request
from dataclasses import asdict, dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
USER_AGENT = "banexg-api-doc-sync/1.0"
SOURCES = {
    "binance": "https://developers.binance.com/en/docs/llms-full.txt",
    "okx": "https://my.okx.com/docs-v5/en/",
    "bybit": "https://codeload.github.com/bybit-exchange/docs/tar.gz/refs/heads/master",
}

DIRECT_ENTRY_RE = re.compile(
    r"(?m)^\s*(Method[A-Za-z0-9_]+):\s*\{Path:\s*\"([^\"]+)\",\s*"
    r"Host:\s*([A-Za-z0-9_\.]+),\s*Method:\s*\"([A-Z]+)\""
)
HELPER_ENTRY_RE = re.compile(
    r"(?m)^\s*(Method[A-Za-z0-9_]+):\s*api\(\"([^\"]+)\",\s*"
    r"([A-Za-z0-9_\.]+),\s*\"([A-Z]+)\""
)
METHOD_RE = re.compile(r"\bMethod[A-Za-z0-9_]+\b")


@dataclass(frozen=True)
class Endpoint:
    exchange: str
    name: str
    method: str
    path: str
    host: str

    @property
    def full_path(self) -> str:
        path = self.path.lstrip("/")
        if self.exchange == "okx":
            return "/api/v5/" + path
        if self.exchange == "bybit":
            return "/" + path
        prefixes = {
            "HostPublic": "/api/v3/",
            "HostPrivate": "/api/v3/",
            "HostV1": "/api/v1/",
            "HostSApi": "/sapi/v1/",
            "HostSApiV2": "/sapi/v2/",
            "HostSApiV3": "/sapi/v3/",
            "HostSApiV4": "/sapi/v4/",
            "HostFApiPublic": "/fapi/v1/",
            "HostFApiPrivate": "/fapi/v1/",
            "HostFApiPublicV2": "/fapi/v2/",
            "HostFApiPrivateV2": "/fapi/v2/",
            "HostDApiPublic": "/dapi/v1/",
            "HostDApiPrivate": "/dapi/v1/",
            "HostDApiPrivateV2": "/dapi/v2/",
            "HostEApiPublic": "/eapi/v1/",
            "HostEApiPrivate": "/eapi/v1/",
            "HostPApi": "/papi/v1/",
        }
        return prefixes.get(self.host, "/") + path


def production_method_refs(exchange_dir: Path) -> set[str]:
    refs: set[str] = set()
    for path in exchange_dir.glob("*.go"):
        if path.name.endswith("_test.go") or path.name in {"entry.go", "data.go"}:
            continue
        refs.update(METHOD_RE.findall(path.read_text(encoding="utf-8")))
    return refs


def endpoint_inventory(root: Path, exchange: str) -> list[Endpoint]:
    exchange_dir = root / exchange
    entry_text = (exchange_dir / "entry.go").read_text(encoding="utf-8")
    declared = {}
    for match in (*DIRECT_ENTRY_RE.finditer(entry_text), *HELPER_ENTRY_RE.finditer(entry_text)):
        name, path, host, method = match.groups()
        declared[name] = Endpoint(exchange, name, method, path, host)
    refs = production_method_refs(exchange_dir)
    return sorted((declared[name] for name in refs if name in declared), key=lambda item: item.name)


def fetch(url: str) -> bytes:
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=120) as response:
        return response.read()


def markdown_chunks(text: str) -> list[str]:
    starts = [match.start() for match in re.finditer(r"(?m)^#{1,4}\s+", text)]
    if not starts:
        return [text]
    starts.append(len(text))
    return [text[starts[i] : starts[i + 1]] for i in range(len(starts) - 1)]


def extract_binance(data: bytes, endpoints: list[Endpoint]) -> tuple[str, set[str]]:
    text = data.decode("utf-8", errors="replace")
    chunks = markdown_chunks(text)
    matched: set[str] = set()
    selected: list[str] = []
    for chunk in chunks:
        chunk_paths = []
        for endpoint in endpoints:
            if endpoint.full_path in chunk and re.search(rf"\b{endpoint.method}\b", chunk):
                matched.add(endpoint.name)
                chunk_paths.append(endpoint.full_path)
        ws_related = any(token in chunk for token in (
            "@depth", "@trade", "@kline", "@markPrice", "User Data Stream",
            "userDataStream.subscribe", "executionReport", "ORDER_TRADE_UPDATE",
        ))
        error_related = bool(re.match(r"#{1,3}\s+(Error Codes|Errors)", chunk, re.I))
        if chunk_paths or ws_related or error_related:
            selected.append(chunk)
    return "\n\n".join(dict.fromkeys(selected)), matched


def extract_okx(data: bytes, endpoints: list[Endpoint]) -> tuple[str, set[str]]:
    text = data.decode("utf-8", errors="replace")
    starts = [match.start() for match in re.finditer(r"<h3\b", text, re.I)]
    starts.append(len(text))
    chunks = [text[starts[i] : starts[i + 1]] for i in range(len(starts) - 1)]
    matched: set[str] = set()
    selected: list[str] = []
    ws_tokens = (
        ">trades<", ">books<", ">books5<", ">mark-price<", ">candle",
        ">balance_and_position<", ">positions<", ">orders<", ">orders-algo<",
    )
    for chunk in chunks:
        keep = False
        unescaped = html.unescape(chunk)
        for endpoint in endpoints:
            request_line = f"{endpoint.method} {endpoint.full_path}"
            if request_line in unescaped:
                matched.add(endpoint.name)
                keep = True
        heading = chunk[:500].lower()
        if any(token in unescaped for token in ws_tokens) or "error-code" in heading:
            keep = True
        if keep:
            selected.append(chunk)
    return "\n".join(dict.fromkeys(selected)), matched


def safe_tar_members(data: bytes):
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for member in archive.getmembers():
            if not member.isfile() or "../" in member.name:
                continue
            fileobj = archive.extractfile(member)
            if fileobj is not None:
                yield member.name, fileobj.read()


def extract_bybit(data: bytes, endpoints: list[Endpoint]) -> tuple[str, set[str]]:
    endpoint_re = re.compile(r'<APIEndpoint\s+method="([A-Z]+)"\s+url="([^"]+)"')
    matched: set[str] = set()
    selected: list[str] = []
    ws_files = {
        "orderbook.mdx", "trade.mdx", "kline.mdx", "ticker.mdx", "wallet.mdx",
        "position.mdx", "execution.mdx",
    }
    for name, raw in safe_tar_members(data):
        if "/docs/v5/" not in name or not name.endswith(".mdx"):
            continue
        text = raw.decode("utf-8", errors="replace")
        requests = set(endpoint_re.findall(text))
        file_name = Path(name).name
        is_ws = "/websocket/" in name and file_name in ws_files
        is_error = file_name == "error.mdx"
        file_matches = []
        for endpoint in endpoints:
            if (endpoint.method, endpoint.full_path) in requests:
                matched.add(endpoint.name)
                file_matches.append(endpoint.name)
        if file_matches or is_ws or is_error:
            selected.append(f"\n<!-- source: {name} -->\n{text}")
    return "\n".join(selected), matched


EXTRACTORS = {
    "binance": (extract_binance, "official.md"),
    "okx": (extract_okx, "official.html"),
    "bybit": (extract_bybit, "official.mdx"),
}


def sync_exchange(root: Path, output: Path, exchange: str) -> dict:
    endpoints = endpoint_inventory(root, exchange)
    source_url = SOURCES[exchange]
    raw = fetch(source_url)
    extractor, output_name = EXTRACTORS[exchange]
    content, matched = extractor(raw, endpoints)
    target = output / exchange
    target.mkdir(parents=True, exist_ok=True)
    (target / output_name).write_text(content, encoding="utf-8")
    inventory = [asdict(endpoint) | {"full_path": endpoint.full_path} for endpoint in endpoints]
    (target / "inventory.json").write_text(
        json.dumps(inventory, indent=2, ensure_ascii=True) + "\n", encoding="utf-8"
    )
    unmatched = [endpoint.name for endpoint in endpoints if endpoint.name not in matched]
    return {
        "exchange": exchange,
        "source": source_url,
        "source_sha256": hashlib.sha256(raw).hexdigest(),
        "output_sha256": hashlib.sha256(content.encode()).hexdigest(),
        "endpoint_count": len(endpoints),
        "matched_count": len(matched),
        "unmatched": unmatched,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exchange", choices=sorted(SOURCES), action="append")
    parser.add_argument("--output", type=Path, default=ROOT / "docs" / "api_docs")
    parser.add_argument("--strict", action="store_true", help="fail when a REST route is absent upstream")
    parser.add_argument("--inventory-only", action="store_true")
    args = parser.parse_args(argv)
    exchanges = args.exchange or sorted(SOURCES)
    args.output.mkdir(parents=True, exist_ok=True)
    results = []
    for exchange in exchanges:
        if args.inventory_only:
            endpoints = endpoint_inventory(ROOT, exchange)
            results.append({
                "exchange": exchange,
                "endpoint_count": len(endpoints),
                "endpoints": [asdict(item) | {"full_path": item.full_path} for item in endpoints],
            })
        else:
            results.append(sync_exchange(ROOT, args.output, exchange))
    manifest = {
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "results": results,
    }
    (args.output / "manifest.json").write_text(
        json.dumps(manifest, indent=2, ensure_ascii=True) + "\n", encoding="utf-8"
    )
    print(json.dumps(manifest, indent=2, ensure_ascii=True))
    if args.strict and any(result.get("unmatched") for result in results):
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
