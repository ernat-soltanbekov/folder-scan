#!/usr/bin/env python3
"""Compare compiled folder-scan with native ls; fixtures never touch user files."""

import difflib
import os
from pathlib import Path
import platform
import re
import resource
import socket
import subprocess
import tempfile
import threading
import time

ROOT = Path(__file__).resolve().parent.parent
BINARY = ROOT / "folder-scan"
ENV = dict(os.environ)
for key in ("CLICOLOR", "CLICOLOR_FORCE", "COLORTERM", "LS_COLORS", "LSCOLORS", "BLOCKSIZE", "BLOCK_SIZE", "LS_BLOCK_SIZE", "TIME_STYLE", "QUOTING_STYLE", "POSIXLY_CORRECT"):
    ENV.pop(key, None)
ENV.update(LC_ALL="C", TZ="UTC")
CHECKS = 0


def run(args, cwd, native=False, timeout=20):
    return subprocess.run(["/bin/ls" if native else str(BINARY), *args], cwd=cwd,
                          env=ENV, capture_output=True, text=True, timeout=timeout)


def compare(args, cwd):
    global CHECKS
    expected = run(args, cwd, native=True)
    actual = run(["--no-profile", *args], cwd)
    assert expected.returncode == actual.returncode == 0, (args, expected.stderr, actual.stderr)
    assert actual.stderr == "", (args, actual.stderr)
    if expected.stdout != actual.stdout:
        diff = "".join(difflib.unified_diff(expected.stdout.splitlines(True), actual.stdout.splitlines(True), fromfile="system ls", tofile="folder-scan"))
        raise AssertionError(f"{args}:\n{diff[:6000]}")
    CHECKS += 1


def fixtures(root):
    for name, size in (("alpha", 0), ("Beta", 12), ("zeta", 10001), ("space name", 5),
                       (".hidden", 2), ("кириллица", 4), ("-option", 1)):
        (root / name).write_bytes(b"x" * size)
    (root / "run").write_text("#!/bin/sh\n")
    (root / "run").chmod(0o755)
    (root / "setuid").write_text("x")
    (root / "setuid").chmod(0o4644)
    for name in ("dir", "dir/nested", "empty", "-", ".hidden-dir"):
        (root / name).mkdir()
    (root / "dir/child").write_text("content")
    (root / "dir/nested/grandchild").write_text("content")
    (root / "-/inside").write_text("dash directory")
    os.link(root / "Beta", root / "hardlink")
    for name, target in (("file-link", "Beta"), ("dir-link", "dir"), ("broken", "missing")):
        (root / name).symlink_to(target)
    os.mkfifo(root / "fifo", 0o600)
    local_socket = socket.socket(socket.AF_UNIX)
    local_socket.bind(str(root / "socket"))
    # The nanosecond ties deliberately test the reverse lexical tie breaker.
    now = time.time_ns()
    for index, name in enumerate(("alpha", "Beta", "zeta", "space name", "run")):
        stamp = now - (index // 2) * 86400 * 10**9
        os.utime(root / name, ns=(stamp, stamp))
    return local_socket


def listing_cases(root):
    for flag in ("", "-l", "-a", "-r", "-t", "-la", "-latr", "-R", "-lRr", "-alRrt",
                 "-A", "-d", "-ld", "-n", "-ln", "-i", "-li", "-S", "-lS", "-F", "-p", "-k", "-lk", "-l1", "-1l"):
        compare([flag] if flag else [], root)
        compare(([flag] if flag else []) + ["dir"], root)
    for mask in range(32):
        flags = "".join(flag for index, flag in enumerate("lartR") if mask & (1 << index))
        compare(["-" + flags] if flags else [], root)
    for args in (["-l", "-t", "dir"], ["-"], ["-l", "-"], ["--", "-option"],
                 ["-l", "Beta"], ["-l", "file-link"], ["-l", "dir-link"], ["-l", "dir-link/"],
                 ["file-link"], ["dir-link"], ["-F", "dir-link"], ["-H", "dir-link"], ["-lH", "dir-link"],
                 ["-L", "file-link"], ["-l", "broken"],
                 ["-l", "Beta", "empty", "dir", "alpha"], ["-R", "empty", "dir"],
                 ["-l", "empty"], ["-alRrt", "empty"], ["-l", "dir/", "dir-link/"]):
        compare(args, root)
    if platform.system() == "Darwin":
        compare(["-P", "dir-link"], root)
    else:
        physical = run(["-P", "dir-link"], root)
        assert physical.returncode == 0 and physical.stdout == "dir-link\n"
    for args in (["-l", "file-link/"], ["missing"], ["-L", "broken"], ["-z"], ["--color=wrong"]):
        result = run(args, root)
        assert result.returncode != 0 and result.stderr and not result.stdout, (args, result)
    result = run(["missing", "Beta"], root)
    assert result.returncode != 0 and result.stdout == "Beta\n" and "missing" in result.stderr
    for name in ("line\nbreak", "tab\tname", "quote'and\"double"):
        (root / name).touch()
    compare(["-l"], root)
    for name in ("line\nbreak", "tab\tname", "quote'and\"double"):
        (root / name).unlink()


def profiles(root):
    path = root / "ages"
    path.mkdir()
    for flags in ([], ["-l"], ["-a"]):
        assert "Age profile:" not in run([*flags, "ages"], root).stdout
    now = time.time()
    for index, days in enumerate((0, 6, 7, 30, 31, 365, 366, 1000)):
        target = path / str(index)
        target.touch()
        stamp = now - days * 86400
        os.utime(target, (stamp, stamp))
    result = run(["ages"], root)
    assert result.returncode == 0 and result.stdout.endswith("Age profile: fresh 2 | recent 2 | aging 2 | archived 2\n"), result.stdout
    assert "Age profile:" not in run(["ages/0"], root).stdout
    assert "Age profile:" not in run(["-d", "ages"], root).stdout
    result = run(["-R", "dir"], root)
    assert result.stdout.count("Age profile:") == 2, result.stdout
    result = run(["--color=always", "-F", "dir", "run"], root)
    assert "\x1b[" in result.stdout and "run\x1b[0m*" in result.stdout
    result = run(["--color=auto", "dir"], root)
    assert "\x1b[" not in result.stdout
    date_probe = root / "date-probe"
    date_probe.touch()
    for days in (0, 181, 182, 183, 400, -1):
        stamp = time.time() - days * 86400
        os.utime(date_probe, (stamp, stamp))
        compare(["-l", "date-probe"], root)


def failure_cases(root):
    # Cycle detection must not suppress a separate alias of the same directory.
    (root / "dir/nested/back").symlink_to("..")
    result = run(["-RL", "dir", "dir-link"], root, timeout=5)
    assert result.returncode != 0 and "recursive directory loop" in result.stderr
    assert "dir-link:" in result.stdout
    (root / "dir/nested/back").unlink()
    blocked = root / "blocked"
    blocked.mkdir()
    (blocked / "secret").touch()
    blocked.chmod(0)
    try:
        if os.geteuid() != 0:
            result = run(["-R", "blocked", "dir"], root)
            assert result.returncode != 0 and "blocked" in result.stderr and "child" in result.stdout
    finally:
        blocked.chmod(0o700)
    changing = root / "changing"
    changing.mkdir()
    stop = threading.Event()

    def mutate():
        while not stop.is_set():
            for index in range(30):
                path = changing / str(index)
                path.touch()
                path.unlink()

    worker = threading.Thread(target=mutate)
    worker.start()
    try:
        for _ in range(20):
            result = run(["-lR", "changing"], root, timeout=5)
            assert result.returncode in (0, 1, 2) and "panic:" not in result.stderr
    finally:
        stop.set()
        worker.join(timeout=5)
    deep = root / "deep"
    deep.mkdir()
    cursor = deep
    for _ in range(256):
        cursor = cursor / "d"
        cursor.mkdir()
    (cursor / "end").touch()

    def low_descriptor_limit():
        resource.setrlimit(resource.RLIMIT_NOFILE, (32, 32))

    result = subprocess.run([str(BINARY), "-R", "deep"], cwd=root, env=ENV,
                            capture_output=True, text=True, timeout=10,
                            preexec_fn=low_descriptor_limit)
    assert result.returncode == 0 and result.stderr == "" and "end\n" in result.stdout
    assert result.stdout.count("Age profile:") == 257


def system_directories():
    compare(["-l", "/usr/bin"], ROOT)
    # /dev is live: on macOS even lstat of a clone device changes its number.
    # Compare stable entries byte-for-byte; identify unstable lines by taking
    # TWO independent native ls snapshots, never by a hardcoded exemption list.
    before = run(["-la", "/dev"], ROOT, native=True)
    actual = run(["--no-profile", "-la", "/dev"], ROOT)
    after = run(["-la", "/dev"], ROOT, native=True)
    assert before.returncode == actual.returncode == after.returncode == 0

    def entries(text):
        result = {}
        for line in text.splitlines():
            fields = line.split(None, 8 if platform.system() == "Darwin" else 8)
            if len(fields) < 9:
                continue
            # Device major/minor occupies two fields on Linux. Use the final
            # date/time columns to locate the name without normalizing spacing.
            match = re.search(r"\b[A-Z][a-z]{2}\s+\d+\s+(?:\d{2}:\d{2}|\d{4})\s+(.*)$", line)
            if match:
                # The audit explicitly permits ignoring the ACL marker.
                normalized = line[:10] + (" " if line[10:11] in "+@." else line[10:11]) + line[11:]
                result[match.group(1)] = normalized
        return result

    a, b, c = entries(before.stdout), entries(actual.stdout), entries(after.stdout)
    stable = {name for name in a.keys() & c.keys() if a[name] == c[name]}
    assert stable and len(stable) >= len(a) * 0.8
    for name in stable:
        assert b.get(name) == a[name], (name, a[name], b.get(name))
    print(f"/dev: {len(stable)} stable entries matched; {len(a)-len(stable)} changed between native snapshots")


def attributes(root):
    path = root / "alpha"
    if platform.system() == "Darwin":
        subprocess.run(["/usr/bin/xattr", "-w", "com.example.folder-scan", "test", str(path)], check=True)
        try:
            compare(["-l", "alpha"], root)
        finally:
            subprocess.run(["/usr/bin/xattr", "-d", "com.example.folder-scan", str(path)], check=True)
    elif hasattr(os, "setxattr"):
        os.setxattr(path, "user.folder-scan", b"test")
        try:
            compare(["-l", "alpha"], root)
        finally:
            os.removexattr(path, "user.folder-scan")


if __name__ == "__main__":
    with tempfile.TemporaryDirectory(prefix="folder-scan-audit-") as directory:
        root = Path(directory)
        sock = fixtures(root)
        try:
            listing_cases(root)
            profiles(root)
            failure_cases(root)
            attributes(root)
            system_directories()
        finally:
            sock.close()
    print(f"PASS: {CHECKS} exact native-ls comparisons, age boundaries, colors, recursion, links, permissions and concurrent mutation")
