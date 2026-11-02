"""Private synthetic metadata acceptance and bounded load runner (stdlib only)."""

import argparse
import concurrent.futures
import datetime
import json
import math
import pathlib
import ssl
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


MAX_RESPONSE_BYTES = 16 * 1024 * 1024
MUSIC_ID = "00000000-0000-4000-8000-000000000001"
TLS_CONTEXT = ssl.create_default_context()
HTTP_STATE = threading.local()


def valid_envelope(body, provider):
    if not isinstance(body, dict):
        return False
    if body.get("schema_version") != 1 or body.get("source") != provider:
        return False
    if body.get("completeness") != "complete" or not isinstance(body.get("stale"), bool):
        return False
    if not isinstance(body.get("data"), dict) or "source_id" not in body:
        return False
    try:
        for field in ("fetched_at", "expires_at"):
            stamp = datetime.datetime.fromisoformat(body[field].replace("Z", "+00:00"))
            if stamp.tzinfo is None:
                return False
    except (KeyError, TypeError, ValueError, AttributeError):
        return False
    return True


def percentile(values, percent):
    if not values:
        return None
    ordered = sorted(values)
    return round(ordered[max(0, math.ceil(len(ordered) * percent / 100) - 1)] * 1000, 3)


def summarize(results, elapsed):
    successful = [r["seconds"] for r in results if 200 <= r["status"] < 300 and r["valid"]]
    counts = {}
    for result in results:
        status = str(result["status"])
        counts[status] = counts.get(status, 0) + 1
    return {
        "requests": len(results),
        "success": len(successful),
        "non_2xx": sum(not 200 <= r["status"] < 300 for r in results),
        "expected_429": sum(r["status"] == 429 for r in results),
        "unexpected_non_2xx": sum(not 200 <= r["status"] < 300 and r["status"] != 429 for r in results),
        "server_errors": sum(500 <= r["status"] < 600 for r in results),
        "transport_errors": sum(r["status"] == 0 for r in results),
        "invalid_success": sum(200 <= r["status"] < 300 and not r["valid"] for r in results),
        "status_counts": counts,
        "elapsed_seconds": round(elapsed, 6),
        "throughput_per_second": round(len(results) / elapsed, 3) if elapsed else 0,
        "success_throughput_per_second": round(len(successful) / elapsed, 3) if elapsed else 0,
        "latency_success_ms": {"p50": percentile(successful, 50), "p95": percentile(successful, 95), "p99": percentile(successful, 99)},
        "latency_all_ms": {"p50": percentile([r["seconds"] for r in results], 50), "p95": percentile([r["seconds"] for r in results], 95), "p99": percentile([r["seconds"] for r in results], 99)},
    }


def make_routes(mode, count):
    if mode == "dataset":
        return [f"/v1/movies/tmdb/{i}" for i in range(1, count + 1)]
    if mode == "cold":
        return [f"/v1/movies/tmdb/{500000000 + i}" for i in range(1, count + 1)]
    return [
        "/v1/movies/tmdb/900001",
        "/v1/shows/tmdb/900002",
        "/v1/shows/tvmaze/900003",
        f"/v1/music/musicbrainz/release/{MUSIC_ID}",
    ]


def request_budget(rate, duration, count):
    if not math.isfinite(rate) or not math.isfinite(duration) or rate <= 0 or duration <= 0 or count < 1:
        raise ValueError("rate, duration and count must be positive finite bounds")
    return min(count, math.ceil(rate * duration))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def thread_opener():
    if not hasattr(HTTP_STATE, "opener"):
        HTTP_STATE.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPSHandler(context=TLS_CONTEXT),
            NoRedirect(),
        )
    return HTTP_STATE.opener


def exchange(base, route, token=None, method="GET", body=None, timeout=30):
    headers = {"Accept": "application/json"}
    if token is not None:
        headers["Authorization"] = "Bearer " + token
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(base.rstrip("/") + route, data=data, headers=headers, method=method)
    started = time.monotonic()
    status, parsed, response_headers = 0, None, {}
    try:
        opener = thread_opener()
        try:
            response = opener.open(req, timeout=timeout)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.code
            response_headers = {k.lower(): v for k, v in response.headers.items()}
            raw = response.read(MAX_RESPONSE_BYTES + 1)
            if len(raw) <= MAX_RESPONSE_BYTES:
                try:
                    parsed = json.loads(raw)
                except (UnicodeDecodeError, json.JSONDecodeError):
                    pass
    except (OSError, urllib.error.URLError, TimeoutError):
        pass
    return {"status": status, "seconds": time.monotonic() - started, "body": parsed, "headers": response_headers}


def provider_for(route):
    if route.startswith("/v1/search?"):
        return urllib.parse.parse_qs(urllib.parse.urlsplit(route).query).get("provider", [""])[0]
    parts = route.split("/")
    return parts[3] if len(parts) > 3 else ""


def lookup(base, route, token, timeout=30):
    answer = exchange(base, route, token, timeout=timeout)
    valid = valid_envelope(answer["body"], provider_for(route))
    if valid and route.startswith("/v1/search?"):
        valid = isinstance(answer["body"]["data"].get("items"), list)
    return {"status": answer["status"], "seconds": answer["seconds"], "valid": valid}


def run_bounded(base, routes, token, rate, duration, count, concurrency, timeout=30):
    budget = request_budget(rate, duration, count)
    started = time.monotonic()
    deadline, next_at = started + duration, started
    results, inflight = [], set()
    sent = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        while sent < budget and time.monotonic() < deadline:
            if len(inflight) >= concurrency:
                done, inflight = concurrent.futures.wait(inflight, timeout=max(0, deadline - time.monotonic()), return_when=concurrent.futures.FIRST_COMPLETED)
                results.extend(f.result() for f in done)
                if not done:
                    break
            delay = next_at - time.monotonic()
            if delay > 0:
                time.sleep(min(delay, max(0, deadline - time.monotonic())))
            if time.monotonic() >= deadline:
                break
            inflight.add(executor.submit(lookup, base, routes[sent % len(routes)], token, timeout))
            sent += 1
            # Missed slots never produce a catch-up burst or an unbounded executor queue.
            next_at = max(next_at + 1 / rate, time.monotonic() + 1 / rate)
        results.extend(f.result() for f in concurrent.futures.as_completed(inflight))
    summary = summarize(results, time.monotonic() - started)
    summary["request_budget"] = budget
    summary["scheduled_rate_per_second"] = rate
    summary["concurrency_limit"] = concurrency
    return summary


def prefill(base, routes, token, rate, timeout):
    results = []
    started = time.monotonic()
    for index, route in enumerate(routes):
        results.append(lookup(base, route, token, timeout))
        if index + 1 < len(routes):
            time.sleep(1 / rate)
    return summarize(results, time.monotonic() - started)


def fixture_control(base, **settings):
    response = exchange(base, "/control", method="POST", body=settings)
    if response["status"] != 200 or response["body"] != {"ok": True}:
        raise RuntimeError("synthetic fixture control unavailable")


def fixture_stats(base):
    response = exchange(base, "/stats")
    if response["status"] != 200 or not isinstance(response["body"], dict):
        raise RuntimeError("synthetic fixture statistics unavailable")
    return summarize_fixture_stats(response["body"])


def summarize_fixture_stats(stats):
    stamps = stats.get("timestamps") or []
    gaps = []
    for left, right in zip(stamps, stamps[1:]):
        try:
            a = datetime.datetime.fromisoformat(left.replace("Z", "+00:00"))
            b = datetime.datetime.fromisoformat(right.replace("Z", "+00:00"))
            gaps.append((b - a).total_seconds() * 1000)
        except (ValueError, TypeError, AttributeError):
            pass
    provider_stamps = {}
    for route, record in stats.get("routes", {}).items():
        provider = route.strip("/").split("/")[0]
        provider_stamps.setdefault(provider, []).extend(record.get("timestamps", []))
    provider_gaps = {}
    for provider, timestamps in provider_stamps.items():
        ordered = sorted(datetime.datetime.fromisoformat(stamp.replace("Z", "+00:00")) for stamp in timestamps)
        intervals = [(right - left).total_seconds() * 1000 for left, right in zip(ordered, ordered[1:])]
        provider_gaps[provider] = round(min(intervals), 3) if intervals else None
    return {"total": stats.get("total"), "tracked_routes": len(stats.get("routes", {})), "dropped_routes": stats.get("dropped_routes", 0), "timestamps_truncated": stats.get("timestamps_truncated", False), "minimum_arrival_gap_ms": round(min(gaps), 3) if gaps else None, "provider_minimum_arrival_gap_ms": provider_gaps}


def smoke(base, fixture, token, rate, timeout):
    checks = []
    interval = 1 / rate
    def check(name, passed, status=None):
        item = {"name": name, "passed": bool(passed)}
        if status is not None:
            item["status"] = status
        checks.append(item)

    for path in ("/healthz", "/readyz"):
        response = exchange(base, path, timeout=timeout)
        check(path.strip("/"), response["status"] == 200, response["status"])
    for name, credential in (("missing_token", None), ("invalid_token", "invalid-synthetic-credential")):
        response = exchange(base, "/v1/movies/tmdb/700000001", credential, timeout=timeout)
        check(name, response["status"] == 401, response["status"])
    response = exchange(base, "/metrics", timeout=timeout)
    check("metrics_requires_token", response["status"] == 401, response["status"])
    response = exchange(base, "/metrics", token, timeout=timeout)
    check("metrics_authorized", response["status"] == 200, response["status"])

    routes = [
        "/v1/search?provider=tmdb&kind=movie&q=synthetic%20%C3%A9&language=ja&region=JP&page=1&limit=20",
        "/v1/search?provider=tmdb&kind=show&q=synthetic&page=1&limit=20",
        "/v1/search?provider=tvmaze&kind=show&q=synthetic&page=1&limit=20",
        *[f"/v1/search?provider=musicbrainz&kind={kind}&q=synthetic&page=1&limit=20" for kind in ("artist", "release-group", "release", "recording")],
        "/v1/movies/tmdb/700000002?language=ja",
        "/v1/shows/tmdb/700000003",
        "/v1/shows/tmdb/700000003/seasons/0",
        "/v1/shows/tvmaze/700000004",
        *[f"/v1/music/musicbrainz/{kind}/{MUSIC_ID}" for kind in ("artist", "release-group", "release", "recording")],
    ]
    results = []
    started = time.monotonic()
    for index, route in enumerate(routes):
        result = lookup(base, route, token, timeout)
        results.append(result)
        check(f"functional_route_{index + 1}", result["status"] == 200 and result["valid"], result["status"])
        time.sleep(interval)
    functional_elapsed = time.monotonic() - started

    if fixture:
        fixture_control(fixture, reset=True)
        cache_route = "/v1/movies/tmdb/700000101"
        cold = lookup(base, cache_route, token, timeout)
        before = fixture_stats(fixture)["total"]
        time.sleep(interval)
        warm = lookup(base, cache_route, token, timeout)
        after = fixture_stats(fixture)["total"]
        check("warm_cache_avoids_provider", cold["valid"] and warm["valid"] and before == 1 and after == before)
        for offset, mode in enumerate(("malformed", "truncated", "incomplete", "oversized"), 1):
            identifier = 700000110 + offset
            fixture_control(fixture, route=f"/tmdb/3/movie/{identifier}", mode=mode)
            time.sleep(interval)
            broken = lookup(base, f"/v1/movies/tmdb/{identifier}", token, timeout)
            check(f"rejects_{mode}", broken["status"] >= 400 and broken["status"] != 429, broken["status"])
            fixture_control(fixture, route=f"/tmdb/3/movie/{identifier}")
            time.sleep(interval)
            repaired = lookup(base, f"/v1/movies/tmdb/{identifier}", token, timeout)
            check(f"recovers_{mode}_without_bad_cache", repaired["status"] == 200 and repaired["valid"], repaired["status"])
        fixture_control(fixture, reset=True)
    summary = summarize(results, functional_elapsed)
    summary["measurement_scope"] = "functional_routes; auth, cache and injected faults appear in checks"
    summary["checks"] = checks
    summary["passed"] = all(check["passed"] for check in checks)
    return summary


def validate_base(value):
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("base URL must be HTTP(S) without credentials, query or fragment")
    return value.rstrip("/")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--fixture-url")
    parser.add_argument("--token-file", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--mode", choices=("smoke", "load", "cold", "dataset"), default="smoke")
    parser.add_argument("--rate", type=float, default=1)
    parser.add_argument("--duration", type=float, default=30)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--count", type=int, default=100)
    parser.add_argument("--payload-bytes", type=int, default=0)
    parser.add_argument("--timeout", type=float, default=30)
    args = parser.parse_args()
    try:
        base = validate_base(args.base_url)
        fixture = validate_base(args.fixture_url) if args.fixture_url else None
        request_budget(args.rate, args.duration, args.count)
        if not 1 <= args.concurrency <= 256 or not 1 <= args.count <= 100000 or not 0 <= args.payload_bytes <= 8 * 1024 * 1024 or not math.isfinite(args.timeout) or not 0 < args.timeout <= 300:
            raise ValueError("concurrency, count, payload or timeout outside bounded range")
        if args.payload_bytes and not fixture:
            raise ValueError("payload size requires --fixture-url")
        token = pathlib.Path(args.token_file).read_text(encoding="utf-8").strip()
        if not token or "\n" in token or "\r" in token:
            raise ValueError("token file must contain one nonempty credential")
    except (ValueError, OSError):
        parser.error("invalid configuration or inaccessible token file")

    output = {"mode": args.mode, "synthetic_only": True, "requested": {"rate": args.rate, "duration": args.duration, "concurrency": args.concurrency, "count": args.count, "payload_bytes": args.payload_bytes}}
    try:
        if fixture:
            fixture_control(fixture, reset=True, payload_bytes=args.payload_bytes)
        if args.mode == "smoke":
            output["measurement"] = smoke(base, fixture, token, args.rate, args.timeout)
            output["passed"] = output["measurement"]["passed"]
        else:
            routes = make_routes(args.mode, args.count)
            if args.mode == "load":
                output["warm_prefill"] = prefill(base, routes, token, args.rate, args.timeout)
                if output["warm_prefill"]["success"] != len(routes):
                    raise RuntimeError("warm prefill failed; no warm-load claim made")
                if fixture:
                    output["provider_before_measurement"] = fixture_stats(fixture)
            output["measurement"] = run_bounded(base, routes, token, args.rate, args.duration, args.count, args.concurrency, args.timeout)
            measured = output["measurement"]
            output["passed"] = measured["requests"] > 0 and measured["success"] > 0 and measured["unexpected_non_2xx"] == 0 and measured["invalid_success"] == 0
            if args.mode in ("cold", "dataset"):
                output["dataset"] = {"unique_ids_requested": measured["requests"], "complete": measured["success"] == args.count, "requested_rows": args.count}
                output["passed"] = output["passed"] and output["dataset"]["complete"]
        if fixture:
            output["provider_after_measurement"] = fixture_stats(fixture)
            if args.mode == "load":
                output["warm_provider_requests"] = output["provider_after_measurement"]["total"] - output["provider_before_measurement"]["total"]
                output["passed"] = output["passed"] and output["warm_provider_requests"] == 0
    except (RuntimeError, OSError, ValueError, KeyError, TypeError):
        output["passed"] = False
        output["error"] = "acceptance operation failed; inspect aggregate checks and private service state"
    finally:
        if fixture:
            try:
                fixture_control(fixture, reset=True)
            except RuntimeError:
                output["fixture_reset_failed"] = True
                output["passed"] = False
    destination = pathlib.Path(args.output)
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(output, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"mode": args.mode, "passed": output["passed"], "counts": output.get("measurement", {}).get("status_counts", {})}, sort_keys=True))
    return 0 if output["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
