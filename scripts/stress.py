#!/usr/bin/env python3
"""Deterministic tree benchmark. Optional --home reads names/metadata only."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent
BINARY = ROOT / "folder-scan"


def measure(path):
    start = time.perf_counter()
    result = subprocess.run([str(BINARY), "-R", str(path)], stdout=subprocess.DEVNULL,
                            stderr=subprocess.PIPE, timeout=30)
    return {"seconds": round(time.perf_counter()-start, 4), "exit_status": result.returncode,
            "diagnostic_lines": len(result.stderr.splitlines())}


with tempfile.TemporaryDirectory(prefix="folder-scan-stress-") as directory:
    root = Path(directory)
    for number in range(100):
        folder = root / f"section-{number:03}"
        folder.mkdir()
        for value in range(100):
            (folder / f"record-{value:03}").touch()
    runs = [measure(root) for _ in range(3)]
    assert all(run["exit_status"] == 0 and run["diagnostic_lines"] == 0 for run in runs), runs
    # This is the requested performance criterion on a reproducible workload.
    assert max(run["seconds"] for run in runs) < 1.5, runs
    print(json.dumps({"fixture": "100 directories / 10000 files", "runs": runs}))

if "--home" in sys.argv:
    print(json.dumps({"fixture": "actual home directory", "runs": [measure(Path.home()) for _ in range(3)]}))
