(module
  ;; Bump allocator global
  (global $bump (mut i32) (i32.const 64))

  (memory (export "memory") 1)

  ;; Invalid JSON for hooks() return — not a JSON array
  (data (i32.const 0) "not valid json")

  ;; alloc(size) → ptr — required for LoadModule
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; hooks() → packed i64 — returns invalid JSON
  ;; Packed: (0 << 32) | 14
  (func $hooks (export "hooks") (result i64)
    i32.const 0
    i64.extend_i32_u
    i64.const 32
    i64.shl
    i32.const 14
    i64.extend_i32_u
    i64.or
  )
)
