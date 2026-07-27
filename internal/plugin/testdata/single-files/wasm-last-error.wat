(module
  ;; Bump allocator global — starts after data section
  (global $bump (mut i32) (i32.const 128))

  (memory (export "memory") 1)

  ;; Offset 0: last_error() return value — static error message
  (data (i32.const 0) "plugin execution failed: bad input")
  ;; Length = 34 bytes

  ;; ── alloc(size) → ptr ─────────────────────────────────────────
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; ── filter(ptr, len) → packed i64 ────────────────────────────
  ;; Always returns packed 0 (error), so caller checks last_error()
  (func $filter (export "filter") (param $ptr i32) (param $len i32) (result i64)
    i64.const 0
  )

  ;; ── last_error() → packed i64 ────────────────────────────────
  ;; Returns pointer to static error message
  ;; Packed: (0 << 32) | 34
  (func $last_error (export "last_error") (result i64)
    i32.const 0
    i64.extend_i32_u
    i64.const 32
    i64.shl
    i32.const 34
    i64.extend_i32_u
    i64.or
  )
)
