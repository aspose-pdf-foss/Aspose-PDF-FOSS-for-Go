#!/usr/bin/env python3
"""DOCX validation harness (epic pdf-go-7qiu) — local-only, like the
pyHanko/pikepdf PDF harnesses.

Rungs:
  1. ECMA-376 transitional XSD validation of word/document.xml, styles.xml
     and numbering.xml (lxml). Catches exactly the class of bugs Word
     "repairs" silently: element order in pPr/rPr, bad w:val values,
     misplaced sectPr.
  2. python-docx round-trip (opens the package, walks paragraphs) — smoke
     for zip/OPC-level breakage.
  3. Optional: LibreOffice headless render (docx -> pdf) when soffice is on
     PATH — the "does it actually open" oracle.

Usage:  python tools/validate_docx.py file.docx [more.docx | dir ...]

The transitional schemas are fetched once into <script_dir>/../result_files/
docx/xsd/ (gitignored) from the ECMA-376 5th-edition mirror; the xml.xsd
import (which ships without a schemaLocation) is patched to resolve locally.
"""

import glob
import os
import shutil
import subprocess
import sys
import urllib.request
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
XSD_DIR = os.path.join(HERE, "..", "result_files", "docx", "xsd")
SCHEMA_BASE = ("https://raw.githubusercontent.com/QtExcel/ecma-376-5th/master/"
               "ECMA-376/OfficeOpenXML-XMLSchema-Transitional")
SCHEMA_FILES = [
    "wml.xsd", "dml-main.xsd", "dml-picture.xsd",
    "dml-wordprocessingDrawing.xsd", "shared-relationshipReference.xsd",
    "shared-commonSimpleTypes.xsd", "shared-math.xsd",
    "shared-customXmlSchemaProperties.xsd",
]
WML_PARTS = ["word/document.xml", "word/styles.xml", "word/numbering.xml"]


def ensure_schemas():
    os.makedirs(XSD_DIR, exist_ok=True)
    for name in SCHEMA_FILES:
        path = os.path.join(XSD_DIR, name)
        if not os.path.exists(path):
            print("fetching", name)
            urllib.request.urlretrieve(f"{SCHEMA_BASE}/{name}", path)
            patch_xml_import(path)
    xml_xsd = os.path.join(XSD_DIR, "xml.xsd")
    if not os.path.exists(xml_xsd):
        urllib.request.urlretrieve("https://www.w3.org/2001/xml.xsd", xml_xsd)


def patch_xml_import(path):
    with open(path, encoding="utf-8") as f:
        s = f.read()
    patched = s.replace(
        '<xsd:import namespace="http://www.w3.org/XML/1998/namespace"/>',
        '<xsd:import namespace="http://www.w3.org/XML/1998/namespace" '
        'schemaLocation="xml.xsd"/>')
    if patched != s:
        with open(path, "w", encoding="utf-8") as f:
            f.write(patched)


def validate_xsd(schema, fn):
    from lxml import etree
    errs = []
    with zipfile.ZipFile(fn) as z:
        names = set(z.namelist())
        for part in WML_PARTS:
            if part not in names:
                errs.append(f"{part}: missing")
                continue
            doc = etree.fromstring(z.read(part))
            if not schema.validate(doc):
                for e in schema.error_log[:5]:
                    errs.append(f"{part}: {e.message[:200]}")
    return errs


def validate_python_docx(fn):
    try:
        import docx
    except ImportError:
        return ["python-docx not installed (pip install python-docx)"]
    try:
        d = docx.Document(fn)
        _ = [p.text for p in d.paragraphs]
        return []
    except Exception as e:  # noqa: BLE001 — any failure is the finding
        return [f"python-docx: {e!r}"]


def find_soffice():
    p = shutil.which("soffice")
    if p:
        return p
    for cand in (r"C:\Program Files\LibreOffice\program\soffice.exe",
                 r"C:\Program Files (x86)\LibreOffice\program\soffice.exe",
                 "/usr/bin/soffice", "/usr/local/bin/soffice",
                 "/Applications/LibreOffice.app/Contents/MacOS/soffice"):
        if os.path.exists(cand):
            return cand
    return None


def validate_soffice(fn, outdir):
    soffice = find_soffice()
    if not soffice:
        return None  # rung unavailable
    r = subprocess.run(
        [soffice, "--headless", "--convert-to", "pdf", "--outdir", outdir, fn],
        capture_output=True, timeout=120)
    stem = os.path.splitext(os.path.basename(fn))[0]
    out = os.path.join(outdir, stem + ".pdf")
    if r.returncode != 0 or not os.path.exists(out) or os.path.getsize(out) == 0:
        return [f"soffice convert failed rc={r.returncode}: "
                f"{r.stderr.decode('utf-8', 'replace')[:200]}"]
    return []


def main():
    files = []
    for arg in sys.argv[1:]:
        if os.path.isdir(arg):
            files += sorted(glob.glob(os.path.join(arg, "*.docx")))
        else:
            files.append(arg)
    if not files:
        print(__doc__)
        sys.exit(2)

    ensure_schemas()
    from lxml import etree
    schema = etree.XMLSchema(etree.parse(os.path.join(XSD_DIR, "wml.xsd")))

    render_dir = os.path.join(XSD_DIR, "..", "render")
    os.makedirs(render_dir, exist_ok=True)
    bad = 0
    soffice_seen = False
    for fn in files:
        errs = validate_xsd(schema, fn) + validate_python_docx(fn)
        so = validate_soffice(fn, render_dir)
        if so is not None:
            soffice_seen = True
            errs += so
        if errs:
            bad += 1
            print(f"FAIL {fn}")
            for e in errs:
                print("   ", e)
        else:
            print(f"ok   {fn}")
    if not soffice_seen:
        print("(LibreOffice not on PATH — render rung skipped)")
    print(f"{len(files) - bad}/{len(files)} valid")
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
