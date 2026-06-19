#! python3
from getpass import getpass
from hashlib import sha256

try:
    from cryptography.hazmat.primitives.kdf.scrypt import Scrypt
except ModuleNotFoundError:
    print("Package cryptography not installed")
    print("Please run (pip install cryptography)")
    exit()

def compute_key(secret: str) -> str:
    pepper = sha256(b"GitGuardian").digest()
    return (
        Scrypt(salt=pepper, n=2048, r=8, p=1, length=32)
        .derive(secret.encode("utf-8"))
        .hex()
    )

secret = getpass("Please paste your secret here: ")
print("Your hash is: ")
print(compute_key(secret))
