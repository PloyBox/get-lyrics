# betterlyrics

Backed by the Better Lyrics API (`https://lyrics-api.boidu.dev`).

Aggregate adapter: it fronts two distinct upstream endpoints and reports which one served the
result via `Result.SubSource` (along with `FieldSubSource`). The endpoint is selected by the
requested sync level:

```
GET /ttml/getLyrics?s=<song>&a=<author>...   word-level request (SyncWord)
                                             → TTML document, preserved verbatim
                                             SubSource: "ttml"
GET /kugou/getLyrics?s=<song>&a=<author>...  line/plain request (SyncLine/SyncNone)
                                             → LRC-style "lyrics" text; a line request
                                             keeps timestamped lines, otherwise the text
                                             is stripped to plain
                                             SubSource: "kugou"
```

- A word-level request returns the raw TTML string unchanged (whitespace between spans is
  significant) with `Level = SyncWord`.
- Line/plain requests produce exactly one track — `SyncLine` when the response actually
  carries timestamped lines, `SyncNone` otherwise.
- The provider-specific endpoint reports the TTML document under different keys depending on
  deployment: the documented shape uses `ttml`, the live API serves it under `lyrics`. The
  adapter accepts either, preferring the documented one.
- `subSourceTTML`/`subSourceKugou` are the `Result.SubSource` identifiers, mirroring the URL
  path suffix (ttml/kugou) so provenance is self-describing.
- Query encoding: `s` (song) and `a` (author) are always sent; `al` (album) and `d` (duration
  in seconds) refine the match when provided.
- Album and duration refine the match; `--author` is required (the API has no title-only
  query).
- 404 is the API's "no match" signal, surfaced as a not-found error rather than a generic
  HTTP-status failure. 401/429 on uncached songs surface as adapter errors and fail over.
- 10s timeout.
