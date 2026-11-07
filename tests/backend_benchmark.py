#!/usr/bin/env python3
# SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

"""Measure real Miniflux HTTP routes against a synthetic local account."""

import argparse
import base64
import http.client
import json
import math
import os
import platform
import re
import ssl
import statistics
import time
from pathlib import Path
from urllib.parse import urlencode, urlsplit


def summarize(values):
    """Return the middle result and nearest-rank 95th percentile."""
    return {
        "samples": len(values),
        "median_ms": round(statistics.median(values), 3),
        "p95_ms": round(sorted(values)[math.ceil(len(values) * 0.95) - 1], 3),
    }


def expected_lists(entries):
    """Calculate list membership and order from the fixed fixture, not the server."""
    numbers = list(range(entries))
    unread = [n for n in numbers if n % 5 != 0]
    score_key = lambda n: (-(n % 101), -n)
    date_key = lambda n: (n % 86400, -n)
    values = {
        "unread_list": sorted(unread, key=score_key),
        "return_list_request": sorted((n for n in unread if n != 1), key=score_key),
        "history_list": [n for n in numbers if n % 5 == 0],
        "saved_list": sorted((n for n in unread if n % 89 == 0), key=score_key),
        "review_list": sorted(
            unread, key=lambda n: (abs((n * 37 % 10000) / 100 - 50), n % 86400, -n)
        ),
        "search_list": sorted(numbers, key=lambda n: n % 86400),
        "date_sort": sorted(unread, key=date_key),
    }
    return {
        name: {
            "ids": [1000 + n for n in ids[:100]],
            "total": len(ids),
            "allow_search_ties": name == "search_list",
            "entries": entries,
        }
        for name, ids in values.items()
    }


def check_list(text, expected):
    """Reject wrong article IDs, order, page size, totals, and removed previews."""
    ids = [
        int(value)
        for value in re.findall(r'<article\b[^>]*\bdata-id="(\d+)"', text, re.DOTALL)
    ]
    assert len(ids) == len(expected["ids"]) and len(set(ids)) == len(ids)
    total = re.search(r"<title>[^<]*\((\d+)\)", text)
    assert total and int(total.group(1)) == expected["total"]
    if expected["allow_search_ties"]:
        assert all(1000 <= entry_id < 1000 + expected["entries"] for entry_id in ids)
        assert [(entry_id - 1000) % 86400 for entry_id in ids] == [
            (entry_id - 1000) % 86400 for entry_id in expected["ids"]
        ]
    else:
        assert ids == expected["ids"], (ids[:5], expected["ids"][:5])
    assert 'class="item-preview"' not in text
    assert "Benchmark article body." not in text


class Client:
    """Reuse one verified HTTPS connection to a loopback server."""

    def __init__(self, url, ca_file, password):
        self.url = urlsplit(url)
        if (
            self.url.scheme != "https"
            or self.url.hostname not in {"localhost", "127.0.0.1", "::1"}
            or self.url.path not in {"", "/"}
        ):
            raise ValueError(
                "The benchmark requires a loopback HTTPS URL without a path."
            )
        self.connection = http.client.HTTPSConnection(
            self.url.hostname,
            self.url.port,
            context=ssl.create_default_context(cafile=ca_file),
            timeout=60,
        )
        self.cookie = ""
        self.csrf = ""
        self.auth = (
            "Basic " + base64.b64encode(("benchmark:" + password).encode()).decode()
        )

    def request(self, path, data=None, api=False, form=False):
        """Measure the complete response before parsing or correctness checks."""
        headers = {"Cookie": self.cookie, "X-Csrf-Token": self.csrf}
        if api:
            headers["Authorization"] = self.auth
        body = None
        if data is not None:
            body = urlencode(data) if form else json.dumps(data)
            headers["Content-Type"] = (
                "application/x-www-form-urlencoded" if form else "application/json"
            )
        start = time.perf_counter()
        self.connection.request("GET" if data is None else "POST", path, body, headers)
        response = self.connection.getresponse()
        payload = response.read()
        elapsed = (time.perf_counter() - start) * 1000
        for key, value in response.getheaders():
            if key.lower() == "set-cookie":
                self.cookie = value.split(";", 1)[0]
        if response.status not in {200, 201, 204, 302, 303}:
            raise AssertionError(
                f"{path} returned HTTP {response.status}: {payload[:200]!r}"
            )
        return payload.decode(), elapsed, response.status

    def login(self, password):
        """Use the real login route and verify the synthetic account."""
        text, _, _ = self.request("/")
        self.csrf = re.search(r'name="csrf" value="([^"]+)"', text).group(1)
        self.request(
            "/login",
            {"csrf": self.csrf, "username": "benchmark", "password": password},
            form=True,
        )
        text, _, _ = self.request("/unread")
        assert 'data-user-id="' in text and 'name="password"' not in text
        self.csrf = re.search(r'data-csrf-token="([^"]+)"', text).group(1)

    def entry(self, entry_id):
        """Check persisted article state outside the measured request."""
        return json.loads(self.request(f"/v1/entries/{entry_id}", api=True)[0])


def benchmark(args):
    """Run fixed requests and retain every sample and validation result."""
    password = os.environ["BENCHMARK_PASSWORD"]
    client = Client(args.url, args.ca_file, password)
    rows = []
    client.login(password)
    expectations = expected_lists(args.entries)

    def record(name, path, check, payload=None):
        """Validate each measured response before accepting its timing."""
        entry_match = re.search(r"/entry/(\d+)", path)
        if entry_match:
            entry_id = int(entry_match.group(1))
            if entry_id not in original_states:
                original_states[entry_id] = client.entry(entry_id)
        text, elapsed, status = client.request(path, payload)
        try:
            check(text)
        except AssertionError as error:
            raise AssertionError(
                f"The {name} check failed for {path}, HTTP {status}."
            ) from error
        if sample:
            rows.append(
                {
                    "action": name,
                    "path": path,
                    "elapsed_ms": elapsed,
                    "response_bytes": len(text.encode()),
                    "http_status": status,
                }
            )
        return text

    def article_check(text):
        """Require the full article body and its action controls."""
        assert "Benchmark article body." in text
        assert 'data-toggle-status="true"' in text

    def apply(name, patch, expected):
        """Measure the same patch route used by current browser actions."""

        def check(text):
            result = json.loads(text)["entries"][0]
            assert result["result"] in {"applied", "unchanged"}, result
            assert all(result["state"][k] == v for k, v in expected.items()), result

        record(name, "/offline/sync", check, {"entries": [{"entry_id": 1000, **patch}]})
        stored = client.entry(1000)
        assert all(stored[k] == v for k, v in expected.items() if k != "user_tag_ids")

    for sample in range(args.samples + 1):
        original_states = {}
        for name, path in [
            ("unread_list", "/unread"),
            ("history_list", "/history"),
            ("saved_list", "/saved-for-later"),
            ("review_list", "/to-review"),
            ("search_list", "/search?q=benchmark"),
            ("date_sort", "/unread?order=published_at&direction=desc"),
        ]:
            record(
                name, path, lambda text, name=name: check_list(text, expectations[name])
            )
        text = record("open_unread_article", "/unread/entry/1001", article_check)
        assert client.entry(1001)["status"] == "read"
        record(
            "return_list_request",
            "/unread",
            lambda text: check_list(text, expectations["return_list_request"]),
        )
        for name, path in [
            ("open_read_article", "/history/entry/1000"),
            ("open_review_article", "/to-review/entry/1002"),
        ]:
            text = record(name, path, article_check)
        for direction in ["next", "previous"]:
            link = re.search(
                r'<a href="([^"]+)"[^>]*data-page="' + direction + '"', text
            )
            assert link, direction
            path = link.group(1).replace("&amp;", "&")
            text = record(direction + "_article", path, article_check)
        for field, value in [("vote", 1), ("starred", True)]:
            apply(
                field,
                {
                    "base": {field: 0 if field == "vote" else False},
                    "set": {field: value},
                },
                {field: value},
            )
            apply(
                "reset_" + field,
                {
                    "base": {field: value},
                    "set": {field: 0 if field == "vote" else False},
                },
                {field: 0 if field == "vote" else False},
            )
        apply(
            "save_for_later",
            {
                "base": {"status": "read", "saved_for_later": False},
                "set": {"status": "unread", "saved_for_later": True},
            },
            {"status": "unread", "saved_for_later": True},
        )
        apply(
            "mark_read",
            {
                "base": {"status": "unread", "saved_for_later": True},
                "set": {"status": "read", "saved_for_later": False},
            },
            {"status": "read", "saved_for_later": False},
        )
        apply(
            "assign_tag",
            {"base": {}, "set": {}, "add_user_tag_ids": [1]},
            {"user_tag_ids": [1]},
        )
        apply(
            "remove_tag",
            {"base": {}, "set": {}, "remove_user_tag_ids": [1]},
            {"user_tag_ids": []},
        )
        # Restore read-on-view changes so every sample uses the same article set.
        patches = []
        for entry_id, original in original_states.items():
            current = client.entry(entry_id)
            keys = ["status", "saved_for_later"]
            before = {k: current[k] for k in keys}
            after = {k: original[k] for k in keys}
            if before != after:
                patches.append({"entry_id": entry_id, "base": before, "set": after})
        if patches:
            restored = json.loads(
                client.request("/offline/sync", {"entries": patches})[0]
            )
            assert all(result["result"] == "applied" for result in restored["entries"])
        for entry_id, original in original_states.items():
            stored = client.entry(entry_id)
            assert stored["status"] == original["status"]
            assert stored["saved_for_later"] == original["saved_for_later"]
        print(f"Completed sample {sample}/{args.samples}", flush=True)
    result = {
        "label": args.label,
        "image_id": os.environ.get("BENCHMARK_IMAGE_ID"),
        "database_image_id": os.environ.get("BENCHMARK_DATABASE_IMAGE_ID"),
        "host": platform.platform(),
        "python": platform.python_version(),
        "entries": args.entries,
        "page_size": 100,
        "warmup_samples": 1,
        "samples_per_action": args.samples,
        "tls_verified": True,
        "browser_used": False,
        "raw": rows,
        "summary": {
            name: summarize([r["elapsed_ms"] for r in rows if r["action"] == name])
            for name in sorted({r["action"] for r in rows})
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result["summary"], indent=2))
    client.connection.close()


def main():
    """Validate local inputs or run small checks without starting services."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="https://localhost:18550")
    parser.add_argument("--ca-file")
    parser.add_argument("--output", type=Path, default=Path("backend-benchmark.json"))
    parser.add_argument("--label", default="local")
    parser.add_argument("--entries", type=int, default=100000)
    parser.add_argument("--samples", type=int, default=11)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        assert summarize([5, 1, 3, 2, 4]) == {"samples": 5, "median_ms": 3, "p95_ms": 5}
        expected = {"ids": [1001, 1002], "total": 2, "allow_search_ties": False}
        valid = '<title>Unread (2)</title><article data-id="1001"></article><article data-id="1002"></article>'
        check_list(valid, expected)
        for invalid in [
            valid.replace("(2)", "(3)"),
            valid.replace("1002", "1003"),
            valid.replace("1001", "1002"),
            valid.replace('<article data-id="1002"></article>', ""),
        ]:
            try:
                check_list(invalid, expected)
            except AssertionError:
                continue
            raise AssertionError("The benchmark accepted incorrect list results.")
        for url in [
            "https://example.com",
            "http://localhost",
            "https://localhost/private",
        ]:
            try:
                Client(url, None, "unused")
            except ValueError:
                continue
            raise AssertionError("The benchmark accepted an unsafe target.")
        print("Benchmark self-check passed.")
        return
    if (
        not args.ca_file
        or not 1 <= args.samples <= 100
        or not 1000 <= args.entries <= 1000000
    ):
        parser.error(
            "Provide a CA file, 1 through 100 samples, and 1000 through 1000000 entries."
        )
    benchmark(args)


if __name__ == "__main__":
    main()
