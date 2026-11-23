"""End-to-end lab test of in-app updates (#146) with a throwaway test key.

Needs a folder (--lab) holding: updatekey(.exe), test.key (a plain key from
"updatekey keygen -plain -pipe"), and Linux amd64 programs holyarrs-1.2.91,
-1.2.92 and -1.2.94 built with
  -X .../internal/updates.labKey=<test public key line>
  -X .../internal/updates.labHost=holyarrs-upd-files
plus holyarrs-1.2.95, a program that passes "update-check" but exits 1.

Runs the old build (1.2.91) in a container on --port (lab ports 18743-18747
only), serves signed lists and programs from an nginx container named
holyarrs-upd-files on a private network, and checks: the notice shows with
mode Off and nothing is downloaded; Ask me downloads, Update now switches,
Go back returns to the image; Automatic skips a release not marked safe,
installs one marked safe, and goes back by itself from one that does not
start. Everything it makes is removed at the end.
"""
import argparse
import http.cookiejar
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request

NET, FILES, APP, IMG = "holyarrs-upd-net", "holyarrs-upd-files", "holyarrs-upd-app", "holyarrs-upd:old"
OWNER = {"name": "labowner", "password": "a long lab passphrase"}


def sh(*a, check=True):
    return subprocess.run(a, check=check, capture_output=True, text=True)


class App:
    def __init__(self, port):
        self.base = f"http://127.0.0.1:{port}"
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.csrf = ""

    def call(self, method, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, method=method, data=data)
        req.add_header("Content-Type", "application/json")
        if self.csrf:
            req.add_header("X-CSRF-Token", self.csrf)
        try:
            with self.op.open(req, timeout=20) as r:
                t = r.read()
                return r.status, (json.loads(t) if t else {})
        except urllib.error.HTTPError as e:
            t = e.read()
            try:
                return e.code, json.loads(t)
            except ValueError:
                return e.code, {"raw": t[:200].decode(errors="replace")}

    def view(self):
        return self.call("GET", "/api/v1/app/update/install")[1]

    def up(self, timeout=120):
        end = time.time() + timeout
        while time.time() < end:
            try:
                with urllib.request.urlopen(self.base + "/healthz", timeout=3) as r:
                    if r.status < 500:
                        return True
            except Exception:
                pass
            time.sleep(1)
        return False


def publish(lab, files, version, serial, safe=False):
    """A signed list offering version, with its program and signature."""
    uk = os.path.join(lab, "updatekey.exe" if os.name == "nt" else "updatekey")
    key = os.path.join(lab, "test.key")
    dist = os.path.join(lab, "dist")
    shutil.rmtree(dist, ignore_errors=True)
    os.makedirs(dist)
    name = f"holyarrs_{version}_linux_amd64"
    shutil.copy(os.path.join(lab, f"holyarrs-{version}"), os.path.join(dist, name))
    sh(uk, "sign", "-binary", "-key", key, "-in", os.path.join(dist, name), "-out", os.path.join(dist, name + ".minisig"))
    for n in (name, name + ".minisig"):
        shutil.copy(os.path.join(dist, n), os.path.join(files, n))
    v1 = os.path.join(files, "v1")
    p = os.path.join(v1, "manifest.json")
    for n in (p, p + ".minisig"):
        if os.path.exists(n):
            os.remove(n)
    # The tool only lists release hosts; the list is pointed at the lab's
    # plain-http host afterwards (only a lab build accepts it), then signed.
    args = [uk, "list", "-version", version, "-dist", dist, "-serial", str(serial), "-out", p,
            "-base", "https://github.com/HolyArrs/HolyArrs/releases/download/" + version]
    if safe:
        args.append("-auto-safe")
    sh(*args)
    with open(p) as f:
        lst = json.load(f)
    for entry in lst["channels"]["stable"]["files"].values():
        entry["url"] = f"http://{FILES}/{name}"
    with open(p, "w") as f:
        json.dump(lst, f, indent=2)
    sh(uk, "sign", "-key", key, "-in", p, "-out", p + ".minisig")


def wait(cond, what, timeout=180):
    end, last = time.time() + timeout, None
    while time.time() < end:
        try:
            good, last = cond()
            if good:
                return last
        except Exception as e:  # the app is switching
            last = repr(e)
        time.sleep(2)
    raise SystemExit(f"FAIL: {what}: {last}")


def running(app, version):
    return lambda: (lambda v: (v.get("running") == version, v))(app.view())


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--lab", required=True)
    ap.add_argument("--port", type=int, default=18743)
    a = ap.parse_args()
    if not 18743 <= a.port <= 18747:
        raise SystemExit("lab ports 18743-18747 only")
    lab = os.path.abspath(a.lab)
    files = os.path.join(lab, "files")
    shutil.rmtree(files, ignore_errors=True)
    os.makedirs(os.path.join(files, "v1"))
    ctx = os.path.join(lab, "img")
    shutil.rmtree(ctx, ignore_errors=True)
    os.makedirs(os.path.join(ctx, "config"))
    os.makedirs(os.path.join(ctx, "tmp"))
    shutil.copy(os.path.join(lab, "holyarrs-1.2.91"), os.path.join(ctx, "holyarrs"))
    with open(os.path.join(ctx, "Dockerfile"), "w") as f:
        f.write("FROM scratch\nCOPY --chmod=755 holyarrs /holyarrs\nCOPY --chown=1000:1000 config /config\n"
                "COPY --chmod=1777 tmp /tmp\nENV container=oci\nENTRYPOINT [\"/holyarrs\"]\nCMD [\"-config\", \"/config\"]\n")
    passed = []

    def ok(msg):
        passed.append(msg)
        print("PASS", msg, flush=True)

    try:
        sh("docker", "build", "-q", "-t", IMG, ctx)
        sh("docker", "network", "create", NET)
        sh("docker", "run", "-d", "--name", FILES, "--network", NET, "-v", f"{files}:/usr/share/nginx/html:ro", "nginx:alpine")
        sh("docker", "run", "-d", "--name", APP, "--network", NET, "--user", "1000:1000", "--restart", "on-failure:5",
           "-p", f"127.0.0.1:{a.port}:6262", "-e", f"HOLYARRS_UPDATE_LIST=http://{FILES}/v1/manifest.json",
           "-e", "HOLYARRS_RELEASE_FEED=", IMG)
        app = App(a.port)
        if not app.up():
            raise SystemExit("FAIL: the old build did not start")
        for _ in range(120):
            st, b = app.call("POST", "/api/v1/auth/setup", OWNER)
            if st != 503:
                break
            time.sleep(1)
        if st >= 300:
            raise SystemExit(f"setup: {st} {b}")
        app.csrf = b.get("csrf", "")
        app.call("POST", "/api/v1/auth/setup/done", {})
        st, cur = app.call("GET", "/api/v1/settings/health")
        st, b = app.call("PUT", "/api/v1/settings/health", {**cur, "update_check_local": True})
        if st >= 300:
            raise SystemExit(f"allow the local list: {st} {b}")

        # 1. Off (the default): the notice shows, nothing is downloaded.
        v = app.view()
        if v.get("mode") != "off" or not v.get("available"):
            raise SystemExit(f"FAIL: default state {v}")
        publish(lab, files, "1.2.92", 10)
        st, c = app.call("POST", "/api/v1/app/update/check")
        if not (c.get("newer") and c.get("latest") == "1.2.92"):
            raise SystemExit(f"FAIL: notice with Off: {st} {c}")
        time.sleep(5)
        if app.view().get("ready"):
            raise SystemExit("FAIL: Off downloaded the release")
        ok("Off (default): notice for 1.2.92, nothing downloaded")

        # 2. Ask me: downloads and checks, waits, Update now switches.
        st, b = app.call("PUT", "/api/v1/app/update/mode", {"mode": "ask"})
        if st >= 300:
            raise SystemExit(f"FAIL: set Ask me {st} {b}")
        app.call("POST", "/api/v1/app/update/check")
        wait(lambda: (lambda v: (v.get("ready") == "1.2.92", v))(app.view()), "Ask me readies 1.2.92")
        time.sleep(5)
        if app.view().get("running") != "1.2.91":
            raise SystemExit("FAIL: Ask me installed by itself")
        st, b = app.call("POST", "/api/v1/app/update/install")
        if st >= 300:
            raise SystemExit(f"FAIL: Update now {st} {b}")
        wait(running(app, "1.2.92"), "switch to 1.2.92")
        ok("Ask me: 1.2.92 downloaded and checked, waited for Update now, switched in place")

        # 3. Go back: the image's own 1.2.91 runs again.
        time.sleep(40)  # marked healthy first
        st, b = app.call("POST", "/api/v1/app/update/go-back")
        if st >= 300:
            raise SystemExit(f"FAIL: go back {st} {b}")
        wait(running(app, "1.2.91"), "go back to 1.2.91")
        ok("Go back: the image's 1.2.91 runs again")

        # 4. Automatic skips a release not marked safe.
        st, b = app.call("PUT", "/api/v1/app/update/mode", {"mode": "auto"})
        if st >= 300:
            raise SystemExit(f"FAIL: set Automatic {st} {b}")
        publish(lab, files, "1.2.94", 20, safe=False)
        st, c = app.call("POST", "/api/v1/app/update/check")
        if c.get("latest") != "1.2.94":
            raise SystemExit(f"FAIL: notice for 1.2.94 {c}")
        time.sleep(20)
        if app.view().get("running") != "1.2.91":
            raise SystemExit("FAIL: Automatic installed a release not marked safe")
        ok("Automatic: 1.2.94 not marked safe is shown, not installed")

        # 5. Automatic installs a release marked safe.
        publish(lab, files, "1.2.94", 30, safe=True)
        app.call("POST", "/api/v1/app/update/check")
        wait(running(app, "1.2.94"), "Automatic installs 1.2.94")
        ok("Automatic: 1.2.94 marked safe installed by itself")
        time.sleep(40)

        # 6. A release that does not start goes back by itself.
        publish(lab, files, "1.2.95", 40, safe=True)
        app.call("POST", "/api/v1/app/update/check")

        def back():
            v = app.view()
            return v.get("running") == "1.2.94" and "1.2.95" in v.get("launch", {}).get("rollback", ""), v
        v = wait(back, "go back from the broken 1.2.95", timeout=300)
        ok("Rollback: 1.2.95 did not start; back on 1.2.94 by itself (" + v["launch"]["rollback"] + ")")
    finally:
        logs = sh("docker", "logs", APP, check=False)
        with open(os.path.join(lab, "app.log"), "w") as f:
            f.write(logs.stdout + logs.stderr)
        for c in (APP, FILES):
            sh("docker", "rm", "-f", c, check=False)
        sh("docker", "network", "rm", NET, check=False)
        sh("docker", "rmi", "-f", IMG, check=False)
    print(f"{len(passed)}/6 passed")
    sys.exit(0 if len(passed) == 6 else 1)


if __name__ == "__main__":
    main()
