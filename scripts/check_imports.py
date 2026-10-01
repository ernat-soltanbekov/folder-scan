#!/usr/bin/env python3
"""Enforce the assignment's direct Go-import allowlist, including platform files."""
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent.parent
ALLOWED = {"fmt", "os", "os/user", "strconv", "strings", "syscall", "time", "math/rand", "errors", "io/fs"}
count = 0
for path in ROOT.rglob("*.go"):
    text = path.read_text()
    for match in re.finditer(r'(?m)^import\s+(?:\((.*?)\)|([^\n]+))', text, re.S):
        for imported in re.findall(r'"([^"]+)"', match.group(1) or match.group(2)):
            local = imported.startswith("github.com/ernat-soltanbekov/folder-scan/internal/")
            test = path.name.endswith("_test.go") and imported == "testing"
            assert imported in ALLOWED or local or test, (str(path.relative_to(ROOT)), imported)
    count += 1
print(f"PASS: {count} Go files use only allowed direct imports (plus testing in test files)")
