(module
  ;; Bump allocator global — starts after data section
  (global $bump (mut i32) (i32.const 256))

  (memory (export "memory") 1)

  ;; Offset 0: hooks() return value — mixed array with strings and objects
  (data (i32.const 0) "[\"onBuildComplete\",{\"name\":\"onContentTransformed\",\"priority\":10,\"pages\":\"blog/**\",\"data\":[\"navigation\",\"team\"],\"pageFields\":[\"title\",\"url\",\"tags\"]}]")
  ;; Length = 148 bytes

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
    i32.const 148
    i64.extend_i32_u
    i64.or
  )

  ;; ── hook(ptr, len) → packed i64 ──────────────────────────────
  ;; Passthrough: echoes input back unchanged
  (func $hook (export "hook") (param $ptr i32) (param $len i32) (result i64)
    local.get $ptr
    i64.extend_i32_u
    i64.const 32
    i64.shl
    local.get $len
    i64.extend_i32_u
    i64.or
  )
)
