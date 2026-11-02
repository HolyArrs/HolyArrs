import importlib.util
import pathlib
import http.server
import json
import threading
import time
import unittest
import concurrent.futures
import urllib.request
from unittest import mock

SPEC = importlib.util.spec_from_file_location("acceptance", pathlib.Path(__file__).with_name("acceptance.py"))
acceptance = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(acceptance)


class HarnessTests(unittest.TestCase):
    def test_openers_are_reused_per_thread_and_tls_context_is_shared(self):
        barrier = threading.Barrier(2)
        def inspect_thread(_):
            opener = acceptance.thread_opener()
            barrier.wait(timeout=5)
            self.assertIs(opener, acceptance.thread_opener())
            https = [handler for handler in opener.handlers if isinstance(handler, urllib.request.HTTPSHandler)]
            self.assertEqual(len(https), 1)
            self.assertFalse(any(handler.proxies for handler in opener.handlers if isinstance(handler, urllib.request.ProxyHandler)))
            redirects = [handler for handler in opener.handlers if isinstance(handler, urllib.request.HTTPRedirectHandler)]
            self.assertEqual(len(redirects), 1)
            self.assertIsInstance(redirects[0], acceptance.NoRedirect)
            return opener, https[0]._context
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
            left, right = list(executor.map(inspect_thread, range(2)))
        self.assertIsNot(left[0], right[0])
        self.assertIs(left[1], right[1])
        self.assertTrue(left[1].check_hostname)
        self.assertEqual(left[1].verify_mode, acceptance.ssl.CERT_REQUIRED)

    def test_success_latency_excludes_failures(self):
        result = acceptance.summarize([
            {"status": 200, "seconds": 0.4, "valid": True},
            {"status": 200, "seconds": 0.8, "valid": True},
            {"status": 429, "seconds": 0.001, "valid": False},
            {"status": 503, "seconds": 0.002, "valid": False},
            {"status": 200, "seconds": 0.003, "valid": False},
        ], 2.0)
        self.assertEqual(result["success"], 2)
        self.assertEqual(result["expected_429"], 1)
        self.assertEqual(result["server_errors"], 1)
        self.assertEqual(result["invalid_success"], 1)
        self.assertEqual(result["latency_success_ms"]["p50"], 400.0)
        self.assertEqual(result["success_throughput_per_second"], 1.0)
        self.assertEqual(result["unexpected_non_2xx"], 1)

    def test_provider_pacing_is_grouped_before_comparison(self):
        result = acceptance.summarize_fixture_stats({"total": 3, "routes": {
            "/tmdb/3/movie/1": {"count": 1, "timestamps": ["2026-10-15T12:00:00Z"]},
            "/tmdb/3/movie/2": {"count": 1, "timestamps": ["2026-10-15T12:00:01Z"]},
            "/tvmaze/shows/1": {"count": 1, "timestamps": ["2026-10-15T12:00:00.001Z"]},
        }, "timestamps": ["2026-10-15T12:00:00Z", "2026-10-15T12:00:00.001Z", "2026-10-15T12:00:01Z"]})
        self.assertEqual(result["provider_minimum_arrival_gap_ms"]["tmdb"], 1000)
        self.assertIsNone(result["provider_minimum_arrival_gap_ms"]["tvmaze"])

    def test_validation_rejects_partial_and_mismatched_envelopes(self):
        body = {"schema_version": 1, "source": "tmdb", "source_id": "7", "fetched_at": "2026-10-15T12:00:00Z", "expires_at": "2026-10-16T12:00:00Z", "stale": False, "completeness": "complete", "data": {"id": "7"}}
        self.assertTrue(acceptance.valid_envelope(body, "tmdb"))
        body["completeness"] = "partial"
        self.assertFalse(acceptance.valid_envelope(body, "tmdb"))
        body["completeness"] = "complete"
        self.assertFalse(acceptance.valid_envelope(body, "tvmaze"))

    def test_dataset_ids_are_unique_and_bounded(self):
        routes = acceptance.make_routes("dataset", 1000)
        self.assertEqual(len(routes), 1000)
        self.assertEqual(len(set(routes)), 1000)
        self.assertEqual(routes[0], "/v1/movies/tmdb/1")
        self.assertEqual(routes[-1], "/v1/movies/tmdb/1000")

    def test_rate_plan_is_bounded(self):
        self.assertEqual(acceptance.request_budget(3, 2, 100), 6)
        self.assertEqual(acceptance.request_budget(100, 2, 7), 7)
        with self.assertRaises(ValueError):
            acceptance.request_budget(0, 2, 7)

    def test_prefill_has_bounded_spacing_and_authenticated_requests(self):
        arrivals, credentials = [], []
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                arrivals.append(time.monotonic())
                credentials.append(self.headers.get("Authorization"))
                payload = json.dumps({"schema_version": 1, "source": "tmdb", "source_id": "7", "fetched_at": "2026-10-15T12:00:00Z", "expires_at": "2026-10-16T12:00:00Z", "stale": False, "completeness": "complete", "data": {"id": "7"}}).encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with mock.patch.object(acceptance.urllib.request, "build_opener", wraps=urllib.request.build_opener) as build:
                summary = acceptance.prefill(f"http://127.0.0.1:{server.server_port}", ["/v1/movies/tmdb/7"] * 3, "test-credential", 20, 2)
            self.assertLessEqual(build.call_count, 1)
            self.assertEqual(summary["success"], 3)
            self.assertEqual(credentials, ["Bearer test-credential"] * 3)
            self.assertTrue(all(right - left >= 0.04 for left, right in zip(arrivals, arrivals[1:])))
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    unittest.main()
