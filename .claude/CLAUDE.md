# CLAUDE.md

## Coding Style (Go)

- Non-test source files should be at most 500 lines.
- Every non-test source file that defines any functions should have a corresponding `_test.go` file.
- Every exported function, type, constant, and variable should have a godoc-style comment.
- Non-exported identifiers should also have godoc-style comments, unless extremely trivial and obvious from their name.
- Packages should have a godoc-style package comment (in `doc.go` or the primary source file).
- Beyond godoc, comments should explain *why*, not repeat what the code does.
- Tabs are 8 spaces for the purpose of line length. Lines should be kept to a reasonable length (100-150 characters, counting tabs as 8 spaces).

## Testing

- We expect high unit test coverage (90%+).
- We like invariants to be asserted: `if !condition { panic("invariant violated: ...") }`. These branches do not need to be tested — they are invariants.

## Claude Working Directory

- Write plans and memories to the `claude/` directory (not `.claude/`), so they can be committed.

## Best Practices

- Commits should be small and focused. Do one thing at a time (e.g., fix one bug). If another issue is found in the process, do it as a follow-up.
- When in doubt, don't assume or make presumptuous judgements — ask the user.
