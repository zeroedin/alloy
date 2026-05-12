(module
  ;; Bump allocator global
  (global $bump (mut i32) (i32.const 128))

  (memory (export "memory") 1)

  ;; Offset 0: valid hooks() JSON
  (data (i32.const 0) "[\"onContentTransformed\"]")
  ;; Length = 24 bytes

  ;; Offset 32: non-JSON response for hook()
  (data (i32.const 32) "<not valid json>")
  ;; Length = 16 bytes

  ;; alloc(size) → ptr — required for LoadModule
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; hooks() → (ptr, len) — returns valid JSON array
  (func $hooks (export "hooks") (result i32 i32)
    i32.const 0
    i32.const 24
  )

  ;; hook(ptr, len) → (ptr, len) — returns non-JSON bytes
  (func $hook (export "hook") (param $ptr i32) (param $len i32) (result i32 i32)
    i32.const 32
    i32.const 16
  )
)
