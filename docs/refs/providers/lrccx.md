# lrccx

Backed by the legacy lrc.cx lyrics API:

```
GET https://api.lrc.cx/jsonapi?title=<song>&artist=<author>&album=<album>
```

- The response is a score-descending JSON array whose entries carry `title`, `artist`, and an
  `lrc` field (the LRC text). Note the field is named `lrc` (not `lyrics` as the legacy docs
  claim) and may be null for instrumental/missing entries — hence the pointer type.
- `Capabilities` honors author and album, independently of each other.
- The adapter takes the first hit whose `lrc` field is present and non-empty; when every hit
  lacks lyrics it falls back to the top entry so the caller sees a deterministic "no usable
  lyrics" error rather than an index panic.
- Because the API only ever returns LRC-flavoured text, a synced request returns the raw
  response text — but only when it actually contains timestamped lines; otherwise (and for
  plain requests) the text is stripped of `[mm:ss]` timestamps and section tags (`[Verse]`,
  `[!text]`, …) to produce plain lyrics. Unsynced `[!text]` entries therefore fall back to
  plain lyrics, matching the `mock-nosync` semantics at the CLI layer.
- The album value `[Unknown Album]` is treated as empty per the API docs. The `path`
  parameter is intentionally never sent — the CLI has no notion of a local music file.
- 10s timeout.
