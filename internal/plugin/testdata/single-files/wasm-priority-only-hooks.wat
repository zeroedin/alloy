(module
  ;; Bump allocator global — starts after data section
  (global $bump (mut i32) (i32.const 128))

  (memory (export "memory") 1)

  ;; Offset 0: hooks() return value — object with name and priority, no scope
  (data (i32.const 0) "[{\"name\":\"onContentTransformed\",\"priority\":25}]")
  ;; Length = 47 bytes

  ;; ── alloc(size) → ptr ─────────────────────────────────────────
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; ── hooks() → (ptr, len) ─────────────────────────────────────
  (func $hooks (export "hooks") (result i32 i32)
    i32.const 0
    i32.const 47
  )

  ;; ── hook(ptr, len) → (ptr, len) ──────────────────────────────
  ;; Passthrough: echoes input back unchanged
  (func $hook (export "hook") (param $ptr i32) (param $len i32) (result i32 i32)
    local.get $ptr
    local.get $len
  )
)
