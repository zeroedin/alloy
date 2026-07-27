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

  ;; filter using sret ABI: (param sret_ptr ptr len) -> () — should be rejected
  ;; This is what modern Rust/TinyGo produce for tuple returns
  (func $filter (export "filter") (param $sret i32) (param $ptr i32) (param $len i32)
    ;; Write ptr to sret[0] and len to sret[4]
    local.get $sret
    local.get $ptr
    i32.store
    local.get $sret
    i32.const 4
    i32.add
    local.get $len
    i32.store
  )
)
