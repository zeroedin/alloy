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

  ;; hooks() → (ptr, len) — returns invalid JSON
  (func $hooks (export "hooks") (result i32 i32)
    i32.const 0
    i32.const 14
  )
)
