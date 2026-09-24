#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Decode BarcodeField output with a third-party reader (zxing-cpp) and compare.

The Go tests validate the encoders against decoders written from reference
semantics, but none of that is a real scanner. This harness renders every
symbology through the real field -> appearance -> rasterizer pipeline
(_examples/barcode_verify) and asks an independent implementation, zxing-cpp,
to read the pixels back.

    pip install zxing-cpp pillow
    python tools/validate_barcodes.py
"""
import json
import subprocess
import sys
from pathlib import Path

import zxingcpp
from PIL import Image

root = Path(__file__).resolve().parent.parent
out = root / "result_files" / "barcodes"
subprocess.run(["go", "run", "./_examples/barcode_verify", str(out)], cwd=root, check=True)

failures = 0
for item in json.loads((out / "manifest.json").read_text(encoding="utf-8")):
    img = Image.open(out / item["file"]).convert("L")
    results = [r for r in zxingcpp.read_barcodes(img) if r.format.name == item["format"]]
    want = item["value"].encode("utf-8")
    if len(results) != 1:
        print(f"FAIL {item['file']}: {len(results)} {item['format']} symbols decoded")
        failures += 1
    elif results[0].bytes != want:
        print(f"FAIL {item['file']}: decoded {results[0].bytes[:40]!r}..., want {want[:40]!r}...")
        failures += 1
    else:
        print(f"ok   {item['file']}  ({len(want)} bytes)")

print("FAILED" if failures else "all symbols decode correctly", f"({failures} failures)" if failures else "")
sys.exit(1 if failures else 0)
