# lrclib

Backed by the public lrclib.net API. The adapter picks the endpoint based on which `Request`
fields are non-empty:

```
GET /api/search?q=<song>                                    when only Song is set
GET /api/get?track_name=...&artist_name=...&album_name=...&duration=<secs>
                                                            when Song + Author are set
```

- Hits carry `plainLyrics` and `syncedLyrics` (LRC) fields; either may be empty.
- `Capabilities` honors author, album and duration — album and duration only take effect on
  the `/api/get` path (author present); a search-only request drops them, so the fetch layer
  can warn the user that `--album`/`--duration` are being ignored.
- A synced request returns the LRC track when the hit carries one; any other request returns
  the plain track — exactly one lyrics track per `Fetch`, with `Level` declaring which.
- The response shape is dictated by the endpoint, not by sniffing the body: `/api/get`
  returns a single object, `/api/search` an array. Branching uses the same condition
  `endpointFor()`/`buildQuery()` use, so a server that pretty-prints (leading
  whitespace/newline) cannot confuse the two.
- Hit selection follows the request: a `line` request prefers the first hit with non-empty
  `syncedLyrics`, so a plain-only result ranked above it cannot mask the richer track. Every
  other request prefers the first hit with non-empty `plainLyrics` (lrclib commonly returns
  instrumental or synced-only entries at the front), then falls back to index 0.
- No timeout of its own — the fetch layer bounds each call via the context.
