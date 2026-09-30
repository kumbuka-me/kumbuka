# Go documentation check

From the application repository, run `go run ./scripts/check-go-docs` to check function, struct, and field descriptions. To check all sibling Go repositories, run:

```sh
go run ./scripts/check-go-docs . ../cli ../sdk ../plugins ../mailbridge
```

The check parses every authored Go file, including platform-specific files and embedded or anonymous struct fields. It excludes unit test files, generated files, vendored code, and Node dependencies. Function summaries should be a single descriptive line beginning with the function name; further explanation can follow. Comments should explain purpose, ownership, units, defaults, or constraints rather than repeat the declaration.
