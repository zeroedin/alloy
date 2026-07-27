(module
  ;; Bump allocator global
  (global $bump (mut i32) (i32.const 64))

  (memory (export "memory") 1)

  ;; alloc(size) → ptr — required for LoadModule
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; filter using OLD multi-value ABI: (result i32 i32) — should be rejected
  (func $filter (export "filter") (param $ptr i32) (param $len i32) (result i32 i32)
    local.get $ptr
    local.get $len
  )
)
