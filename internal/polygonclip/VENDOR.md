# Clipper sources

Clipper 6.4.2 by Angus Johnson, copyright 2010–2017, under Boost Software
License 1.0 (LICENSE). Source repository, immutable commit and SHA-256 hashes
are recorded in sources.json. clipper.cpp.inc and clipper.hpp are unchanged
upstream files. clipper.cpp only controls compilation of that source.

The Go engine statically compiles these sources through cgo. No external
Clipper library or Python runtime is needed. C++11 support is required.
Only closed polygon expansion is exposed; ownership and exception handling
stay inside bridge.cpp. Results select the largest expanded path.

Implicit floating-point contraction is disabled for Clang, matching the
baseline GCC build on Linux. This prevents half-integer arc samples from
rounding differently on ARM64 and x86-64. A tiny polygon can still differ
by one pixel per coordinate from the original library compiled with FMA.
Full image comparisons are recorded separately in docs/VALIDATION.md.
