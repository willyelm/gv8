# internal/v8

This directory contains the bundled V8 assets used by `gv8`:

- `include/` copied V8 public headers
- `*/libgv8.dylib` or `*/libgv8.so` installed or locally built platform runtime
- `*/icudtl.dat` ICU data file when the target build needs external ICU data
- `VERSION` pinned V8 source and release-bundle version
- `SOURCE_COMMIT` pinned bundled V8 source revision

These assets are internal implementation details of the module.
