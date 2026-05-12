(module
  ;; Bump allocator global — starts after data section
  (global $bump (mut i32) (i32.const 256))

  (memory (export "memory") 1)

  ;; ── Data section ──────────────────────────────────────────────
  ;; Offset 0: hooks() return value — JSON array of hook names
  (data (i32.const 0) "[\"onContentTransformed\"]")
  ;; Length = 24 bytes

  ;; Offset 24: known event name to match against
  (data (i32.const 24) "onContentTransformed")
  ;; Length = 20 bytes

  ;; ── alloc(size) → ptr ─────────────────────────────────────────
  (func $alloc (export "alloc") (param $size i32) (result i32)
    global.get $bump
    global.get $bump
    local.get $size
    i32.add
    global.set $bump
  )

  ;; ── filter(ptr, len) → (ptr, len) ────────────────────────────
  ;; Returns input with length+1
  (func $filter (export "filter") (param $ptr i32) (param $len i32) (result i32 i32)
    local.get $len
    i32.eqz
    if
      i32.const 0
      i32.const 0
      return
    end
    local.get $ptr
    local.get $len
    i32.const 1
    i32.add
  )

  ;; ── hooks() → (ptr, len) ─────────────────────────────────────
  ;; Returns pointer to static JSON: ["onContentTransformed"]
  (func $hooks (export "hooks") (result i32 i32)
    i32.const 0
    i32.const 24
  )

  ;; ── hook(ptr, len) → (ptr, len) ──────────────────────────────
  ;; Searches for "onContentTransformed" in the input.
  ;; If found: echoes back input. If not found: returns (0,0) error.
  (func $hook (export "hook") (param $ptr i32) (param $len i32) (result i32 i32)
    (local $i i32)
    (local $j i32)
    (local $match i32)

    ;; Empty input → error
    local.get $len
    i32.eqz
    if
      i32.const 0
      i32.const 0
      return
    end

    ;; Search for "onContentTransformed" (20 bytes at offset 24) in input
    ;; Outer loop: try each starting position
    (block $not_found
      (block $found
        local.get $len
        i32.const 20
        i32.lt_u
        br_if $not_found

        i32.const 0
        local.set $i

        (block $outer_done
          (loop $outer
            ;; If i > len - 20, we can't match
            local.get $i
            local.get $len
            i32.const 20
            i32.sub
            i32.gt_u
            br_if $outer_done

            ;; Check if input[ptr+i .. ptr+i+20] == data[24..44]
            i32.const 1
            local.set $match
            i32.const 0
            local.set $j

            (block $inner_done
              (loop $inner
                local.get $j
                i32.const 20
                i32.ge_u
                br_if $inner_done

                ;; Compare input[ptr+i+j] with data[24+j]
                local.get $ptr
                local.get $i
                i32.add
                local.get $j
                i32.add
                i32.load8_u

                i32.const 24
                local.get $j
                i32.add
                i32.load8_u

                i32.ne
                if
                  i32.const 0
                  local.set $match
                  br $inner_done
                end

                local.get $j
                i32.const 1
                i32.add
                local.set $j
                br $inner
              )
            )

            local.get $match
            br_if $found

            local.get $i
            i32.const 1
            i32.add
            local.set $i
            br $outer
          )
        )
        br $not_found
      )
      ;; Found: echo back input
      local.get $ptr
      local.get $len
      return
    )

    ;; Not found: return (0,0) error
    i32.const 0
    i32.const 0
  )
)
