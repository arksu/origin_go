"""Run a script through Blender's local MCP add-on (null-delimited JSON)."""
import argparse
import json
from pathlib import Path
import socket


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("script", type=Path)
    parser.add_argument("--timeout", type=float, default=60)
    args = parser.parse_args()
    request = {"type": "execute", "code": args.script.read_text(), "strict_json": True}
    with socket.create_connection(("127.0.0.1", 9876), timeout=args.timeout) as connection:
        connection.sendall(json.dumps(request).encode() + b"\0")
        response = bytearray()
        while b"\0" not in response:
            chunk = connection.recv(65536)
            if not chunk:
                raise ConnectionError("Blender MCP closed before returning a response; inspect scene before retrying")
            response.extend(chunk)
    payload = json.loads(response.split(b"\0", 1)[0])
    print(json.dumps(payload, indent=2))
    if payload.get("status") != "ok":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
