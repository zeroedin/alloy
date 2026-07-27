(module
  ;; Bump allocator global — starts after data section
  (global $bump (mut i32) (i32.const 128))

  (memory (export "memory") 1)

  ;; Offset 0: valid hooks() JSON
  (data (i32.const 0) "[\"onContentTransformed\"]")
  ;; Length = 24 bytes

  ;; Offset 32: non-JSON response for hook()
  (data (i32.const 32) "<not valid json>")
  ;; Length = 16 bytes

  ;; ── alloc(size) → ptr ─────────────────────────────────────────
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; ── hooks() → packed i64 ─────────────────────────────────────
  (func $hooks (export "hooks") (result i64)
    i32.const 0
    i64.extend_i32_u
    i64.const 32
    i64.shl
    i32.const 24
    i64.extend_i32_u
    i64.or
  )

  ;; ── hook(ptr, len) → packed i64 — returns non-JSON bytes ─────
  (func $hook (export "hook") (param $ptr i32) (param $len i32) (result i64)
    i32.const 32
    i64.extend_i32_u
    i64.const 32
    i64.shl
    i32.const 16
    i64.extend_i32_u
    i64.or
  )
)
