// Test fixture: calls __registerHook directly with 1 arg (name only).
// Exercises the 1-arg fallback branch (no priority, no scope) to verify
// duplicate detection fires on both code paths of __registerHook (issue #558).
__registerHook("onContentTransformed");
__registerHook("onContentTransformed");
