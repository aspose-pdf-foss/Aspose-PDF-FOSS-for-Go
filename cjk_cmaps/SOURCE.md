# CJK CMap data provenance

The `*.cmap.gz` files (predefined character-code → CID CMaps) and the
`*.cid2uni.gz` files (per-ordering CID → Unicode tables, derived from each
ordering's `cid2code.txt` UTF-16 column) in this directory are generated from
Adobe's **cmap-resources**:

- https://github.com/adobe-type-tools/cmap-resources

That data is distributed by Adobe under the **BSD 3-Clause License**
(Copyright 1990-2019 Adobe, All rights reserved). The `.cmap.gz` files are
gzip-compressed copies of the original CMap text files (the Adobe copyright
header is preserved inside each); the `.cid2uni.gz` files are a compact binary
re-encoding of the public CID→Unicode mapping from the same resources.

The UTF-16 column leaves out some CIDs that the UTF-32 columns map (Japan1's
JIS X 0213 / JIS2004 alternate glyphs, characters outside the BMP).
`tools/gencid2uni` adds those to an existing table, keeping its entries:

    go run tools/gencid2uni/main.go Japan1 <path>/Adobe-Japan1-7/cid2code.txt

run for Japan1 (Adobe-Japan1-7), GB1 (Adobe-GB1-6) and Korea1 (Adobe-Korea1-2);
CNS1 and KR had nothing to add.

Redistribution here is permitted under the BSD 3-Clause terms. This bundled
data is the only non-MIT-licensed content in the repository; the surrounding Go
code remains MIT-licensed (see `LICENSE` at the repo root).
