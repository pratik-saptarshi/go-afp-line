# Contributing to go-afp-line

## Ground rules

1. **Test-first.** Write the failing test before the implementation. A pull request
   without tests is incomplete.
2. **Keep the subset honest.** The contracted subset is defined in
   `docs/subset-contract.md`. Widening it requires updating the contract *first*;
   code that silently accepts out-of-subset content will be rejected.
3. **Loud failures.** New parsers must return typed `afp.DecodeError`s on malformed
   input — never panic, never silently skip.
4. **License hygiene.** No GPL/AGPL dependencies, transitively. Check with
   `go list -m all` and verify licenses before adding anything.
5. **Synthetic fixtures only.** Never commit production AFP data, even "anonymized".

## Checks (must all pass)

```bash
test -z "$(gofmt -l .)"
go mod tidy -diff
go vet ./...
go test -race ./...
```

## Commit style

Conventional commits: `feat(afp): …`, `fix(line): …`, `docs: …`, `test: …`.

## Fuzzing

If you touch `afp/` parsing code, run the fuzzers before opening a PR:

```bash
go test -fuzz=FuzzDecodeIntroducer -fuzztime=60s ./afp/
go test -fuzz=FuzzDecodeField -fuzztime=60s ./afp/
```

Zero panics, zero failures. Keep any new fuzzer-found corpus seeds.
