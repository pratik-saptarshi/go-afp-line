# Task 0 evidence (recovered 2026-10-13)

Raw research material gathered before the Task 0 coordinator was paused for the
plan-review-integrator readiness pass. The re-dispatched Task 0 should use this
as source material for `docs/subset-contract.md` and `docs/writer-spike.md`.

- `afp-research/afplib/` — yan74/afplib (Java, Apache-2.0): structured-field
  introducer layout, field-type IDs, triplet IDs. See `HelloWorld/hello.afp`
  for a real byte sample and `README.md` for the `5A`-wrapped record framing.
- `afp-research/openafp/` — audreyt/openafp (Haskell, CC0): `docs/OpenAFP.Records.AFP.*.html`
  enumerates every MO:DCA structured-field type with byte layouts — the primary
  source for the in-scope TypeID table. `docs/OpenAFP.Internals.*.html` covers
  EBCDIC conversion.
- `afp-research/parse-afp/` — audreyt/parse-afp (Perl, CC0): round-trip
  object model (parse → mutate → regenerate) — reference for the Tier 1
  byte-identical gate design.
- `writerspike/` — `go.mod`/`go.sum` prepared for the pdfcpu-vs-gofpdf spike;
  spike code was not written yet. Redo the spike per the plan's Task 0 Step 3.

License notes: afplib is Apache-2.0, openafp/parse-afp are CC0. Reference only;
no code is copied into the new module.
