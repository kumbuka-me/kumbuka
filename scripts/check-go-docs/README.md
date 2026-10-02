# Go documentation check

From the application repository, run `go run ./scripts/check-go-docs` to check function, struct, and field descriptions. To check all sibling Go repositories, run:

```sh
go run ./scripts/check-go-docs . ../cli ../sdk ../plugins ../mailbridge
```

The check parses every authored Go file, including platform-specific files and embedded or anonymous struct fields. It excludes unit test files, generated files, vendored code, and Node dependencies.

Every function must have a GoDoc comment whose first prose line begins with the function name. Additional explanation may follow on subsequent lines or in additional paragraphs.

Structs and struct fields must have descriptive comments. Comments should explain purpose, ownership, units, defaults, or constraints rather than merely repeat the declaration.
