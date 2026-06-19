#!/usr/bin/env python3
"""
Loop over KEY=value lines (e.g. from a .env file) and print GitGuardian-style
Scrypt hash for each value. Secrets are read from file or stdin only.
Usage:
  python3 compute_key_batch.py path/to/.env
  python3 compute_key_batch.py < path/to/.env
"""
import re
import sys
from hashlib import sha256

try:
    from cryptography.hazmat.primitives.kdf.scrypt import Scrypt
except ModuleNotFoundError:
    print("Package cryptography not installed")
    print("Please run: pip install cryptography")
    sys.exit(1)


def compute_key(secret: str) -> str:
    pepper = sha256(b"GitGuardian").digest()
    return (
        Scrypt(salt=pepper, n=2048, r=8, p=1, length=32)
        .derive(secret.encode("utf-8"))
        .hex()
    )


def parse_env_line(line: str):
    """Parse a single .env-style line. Returns (key, value) or (None, None) to skip."""
    line = line.strip()
    if not line or line.startswith("#"):
        return None, ""
    m = re.match(r"([A-Za-z_][A-Za-z0-9_]*)=(.*)$", line)
    if not m:
        return None, ""
    key, value = m.group(1), m.group(2).strip()
    if value.startswith('"') and value.endswith('"'):
        value = value[1:-1].replace('\\"', '"')
    elif value.startswith("'") and value.endswith("'"):
        value = value[1:-1].replace("\\'", "'")
    return key, value


def main() -> None:
    if len(sys.argv) > 1:
        path = sys.argv[1]
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                lines = f.readlines()
        except OSError as e:
            print(f"Error reading {path}: {e}", file=sys.stderr)
            sys.exit(1)
    else:
        lines = sys.stdin.readlines()

    for line in lines:
        key, value = parse_env_line(line)
        if key is None:
            continue
        if not value:
            print(f"{key}= (empty, no hash)")
            continue
        h = compute_key(value)
        print(f"{key}={h}")


if __name__ == "__main__":
    main()
