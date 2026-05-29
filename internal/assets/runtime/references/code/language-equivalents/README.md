# Language Equivalents

The ATDD process is language-agnostic; the per-phase docs reference the
*concept* (DSL stub, WIP gate, string field type, …) and let the
agent's dispatch pick up the concrete syntax for the project's language
via the `${language}` placeholder.

The combined multi-language view was previously inlined into every
heavy prompt body — ~30 lines of tables per dispatch — even when the
project only used one language. Now each dispatch reads only its own
slice:

| Language | File |
|----------|------|
| Java | [java.md](java.md) |
| C# (.NET) | [csharp.md](csharp.md) |
| TypeScript | [typescript.md](typescript.md) |

## Adding a language

Drop a new `<language>.md` next to the existing files following the same
section structure (TODO Stubs, WIP Gate, String Field Types, DTO
Boilerplate, Test File Naming, Awaitable ShouldSucceed). Add a row to
the table above. The driver passes the language slug from the project's
stack at dispatch time — no further code wiring is needed.
