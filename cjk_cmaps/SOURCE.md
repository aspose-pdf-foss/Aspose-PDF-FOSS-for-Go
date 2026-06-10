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

Redistribution here is permitted under the BSD 3-Clause terms. This bundled
data is the only non-MIT-licensed content in the repository; the surrounding Go
code remains MIT-licensed (see `LICENSE` at the repo root).
