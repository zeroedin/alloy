---
type: patch
---

The `sort` filter and a collection's `sortBy` now compare two whole numbers exactly, however large. A whole number is an integer or a string of digits only. Before, they compared as floating-point numbers, so whole numbers above 2^53 (9007199254740992) could compare as equal and keep their input order. Any other pair of numbers, such as a fraction and a whole number, compares as before.

With two pages in a `g` collection, in this path order, `collections.g | sort: "weight"` gives:

| `weight` values | Before | Now |
|---|---|---|
| `9007199254740993`, `9007199254740992` | `9007199254740993 9007199254740992` | `9007199254740992 9007199254740993` |
| `"100000000000000000000"`, `"99999999999999999999"` | `100000000000000000000 99999999999999999999` | `99999999999999999999 100000000000000000000` |
| `9007199254740993`, `"9007199254740992"` | `9007199254740993 9007199254740992` | `9007199254740992 9007199254740993` |

A collection with `sortBy: "weight"` gives the same order.
