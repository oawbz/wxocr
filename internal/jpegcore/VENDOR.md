# Bundled JPEG decoder

The decoder is based in part on the work of the Independent JPEG Group and
includes libjpeg-turbo 3.2.0. The upstream licenses are retained in `LICENSE.md`
and `README.ijg`; those notices must accompany redistribution.

`sources.json` pins the release archive, SHA-256, translation units, and hashes
of all retained upstream files. Only decompression and common support code is
compiled. The `tu_*.c` files include upstream translation units without changing
their contents. `jconfig.h`, `jversion.h`, and `jconfigint.h` derive from upstream
CMake configuration with SIMD disabled. The last header uses the compiler's
size macros instead of a machine-specific size and identifies the portable build.

The decoder uses the portable integer slow IDCT and fancy chroma upsampling.
Its source is compiled directly by Go's existing cgo toolchain: consumers do
not install CMake, libjpeg, Python, or another decoder runtime. Per-call heap
state isolates libjpeg errors with setjmp/longjmp entirely inside C. Recoverable
warnings follow upstream recovery; fatal errors return to Go without stderr
output. Input Go memory is borrowed only during the C call; output is copied
into Go-owned NRGBA memory before all native allocations are released.

The wrapper currently accepts 8-bit JPEG, including baseline, progressive,
grayscale, and CMYK/YCCK. CMYK conversion follows OpenCV's integer color path.
JPEG decoding is bounded to 100 million pixels. Runtime architecture support
still follows the OCR core's separately documented validation scope.

The engine applies EXIF IFD0 orientation after decoding. All eight transforms
are covered with both TIFF byte orders and compared to OpenCV reference pixels.
`tools/corpus/restore_jpeg_sources.py` restores the pinned upstream files;
the portable generated configuration and project-owned wrapper remain separate.
