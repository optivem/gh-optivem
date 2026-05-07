# Tree-sitter migration: TypeScript first

Implementation has shipped on `main`; CI parity gate is green. Two
verification gates remain. Java and C# follow-ups are parked in
`plans/deferred/2026050{7-100400-tree-sitter-java,7-100500-tree-sitter-csharp}-migration.md`,
unblocked once Step 1 passes.

- [ ] Step 1: build via `scripts/install.sh` and run a real shop
  WRITE cycle that previously hit the `inputSku` failure with
  `ATDD_VERIFY_VERBOSE=1`; confirm the tracer succeeds with one
  selection per channel, no `WARNING: tracer could not stage`. — ⏳
  Deferred: needs a dev box with a C compiler on `PATH` (see
  `CONTRIBUTING.md` Prerequisites) plus a shop scenario; fires next
  time someone runs verify on a tree-sitter-eligible WRITE.
- [ ] Step 2: validate the Windows release binary is self-contained
  — download from the next goreleaser run, confirm it runs on a
  stock Windows machine (no `libwinpthread-1.dll` etc.). — ⏳
  Deferred: fires on the next release.

When both pass, delete this plan and move the Java/C# plans back to
`plans/`.
