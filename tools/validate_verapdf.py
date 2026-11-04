#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Check ConvertToPDFA / AutoTag / ConvertToPDFUA output with veraPDF.

ValidatePDFA and ValidatePDFUA check the rules this library knows about. veraPDF
is the reference: this harness converts the test documents
(_examples/verapdf_samples) and has veraPDF judge the results. The failures listed
in KNOWN are limits of the converter, each one also reported by our own validators.

    python tools/validate_verapdf.py path/to/verapdf   (or set VERAPDF)
"""
import json
import os
import subprocess
import sys
from pathlib import Path

root = Path(__file__).resolve().parent.parent
out = root / "result_files" / "verapdf"
verapdf = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("VERAPDF", "verapdf")

# sample prefix -> veraPDF flavours to check it against
GROUPS = {"ua_": ["ua1"], "a2a_": ["2a", "ua1"], "a2b_": ["2b"], "a1b_": ["1b"]}

# (sample, flavour) -> rules allowed to fail, and why
KNOWN = {
    ("ua_Binder1", "ua1"): {"7.3-1"},  # figures need alternate text; FillAltTexts writes it
    ("a2a_Binder1", "ua1"): {"7.3-1"},
    ("a2b_alfa", "2b"): {"6.2.4.3-3"},  # DeviceCMYK under sRGB, until ConvertDeviceCMYKToRGB (a2b_alfa_rgb)
    ("a1b_alfa", "1b"): {"6.2.3.3-2"},
    ("a2b_PdfWithAcroForm", "2b"): {"6.2.11.4.1-1"},  # ZapfDingbats: no face to embed
    ("a1b_PdfWithAcroForm", "1b"): {"6.3.4-1", "6.2.3.2-2", "6.3.7-1"},  # same; the source's ICC /N is wrong; PDF/A-1 allows no /Differences
}

subprocess.run(["go", "run", "./_examples/verapdf_samples", str(out)], cwd=root, check=True)

bad = 0
for prefix, flavours in GROUPS.items():
    for flavour in flavours:
        for f in sorted(out.glob(prefix + "*.pdf")):
            r = subprocess.run([verapdf, "-f", flavour, "--format", "json", str(f)], capture_output=True, shell=os.name == "nt")
            job = json.loads(r.stdout.decode("utf-8", "replace"))["report"]["jobs"][0]
            vr = job["validationResult"]
            vr = vr[0] if isinstance(vr, list) else vr
            failed = {
                "%s-%s" % (x["clause"], x["testNumber"]): x["description"]
                for x in vr["details"]["ruleSummaries"]
                if x.get("ruleStatus", x.get("status")) == "FAILED"
            }
            unexpected = set(failed) - KNOWN.get((f.stem, flavour), set())
            print("%-28s %-4s %s" % (f.stem, flavour, "FAIL" if unexpected else "ok" if not failed else "ok (known limits)"))
            for rule in sorted(unexpected):
                print("    %s  %s" % (rule, failed[rule][:110]))
            bad += bool(unexpected)
sys.exit(1 if bad else 0)
