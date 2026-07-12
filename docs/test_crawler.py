import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path

import crawler


class CrawlerTest(unittest.TestCase):
    def test_inventory_excludes_unused_and_tests(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            exg = root / "okx"
            exg.mkdir()
            (exg / "entry.go").write_text(
                'MethodUsed: {Path: "market/ticker", Host: HostPublic, Method: "GET"},\n'
                'MethodUnused: {Path: "market/unused", Host: HostPublic, Method: "GET"},\n'
            )
            (exg / "biz.go").write_text("package okx\nvar _ = MethodUsed\n")
            (exg / "biz_test.go").write_text("package okx\nvar _ = MethodUnused\n")
            inventory = crawler.endpoint_inventory(root, "okx")
            self.assertEqual([item.name for item in inventory], ["MethodUsed"])
            self.assertEqual(inventory[0].full_path, "/api/v5/market/ticker")

    def test_okx_extracts_only_used_request(self):
        endpoints = [crawler.Endpoint("okx", "MethodTicker", "GET", "market/ticker", "HostPublic")]
        source = (
            '<h3 id="ticker">Ticker</h3><p><code>GET /api/v5/market/ticker</code></p>'
            '<h3 id="unused">Unused</h3><p><code>GET /api/v5/market/books</code></p>'
        ).encode()
        content, matched = crawler.extract_okx(source, endpoints)
        self.assertIn("market/ticker", content)
        self.assertNotIn("market/books", content)
        self.assertEqual(matched, {"MethodTicker"})

    def test_bybit_extracts_api_and_error_docs(self):
        endpoint = crawler.Endpoint("bybit", "MethodKline", "GET", "v5/market/kline", "HostPublic")
        archive_data = io.BytesIO()
        with tarfile.open(fileobj=archive_data, mode="w:gz") as archive:
            files = {
                "docs-master/docs/v5/market/kline.mdx": '<APIEndpoint method="GET" url="/v5/market/kline" />',
                "docs-master/docs/v5/market/unused.mdx": '<APIEndpoint method="GET" url="/v5/market/unused" />',
                "docs-master/docs/v5/error.mdx": "# Error Codes",
            }
            for name, content in files.items():
                raw = content.encode()
                info = tarfile.TarInfo(name)
                info.size = len(raw)
                archive.addfile(info, io.BytesIO(raw))
        content, matched = crawler.extract_bybit(archive_data.getvalue(), [endpoint])
        self.assertIn("/v5/market/kline", content)
        self.assertIn("Error Codes", content)
        self.assertNotIn("/v5/market/unused", content)
        self.assertEqual(matched, {"MethodKline"})

    def test_inventory_only_cli_writes_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp)
            self.assertEqual(crawler.main(["--exchange", "okx", "--output", str(output), "--inventory-only"]), 0)
            manifest = json.loads((output / "manifest.json").read_text())
            self.assertEqual(manifest["results"][0]["exchange"], "okx")
            self.assertGreater(manifest["results"][0]["endpoint_count"], 0)


if __name__ == "__main__":
    unittest.main()
