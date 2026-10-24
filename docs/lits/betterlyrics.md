# Better Lyrics API

Free lyrics API with syllable-, word- and line-level timing, powering the Better
Lyrics browser extension.

References:

- <https://docs.betterlyrics.org/> (retrieved 2026-10-24)
- Mirror documentation: <https://lyrics-api-docs.boidu.dev/>
- OpenAPI: <https://api.betterlyrics.org/openapi.json>

Base URLs:

- Official: `https://api.betterlyrics.org`
- Deployment used by the CLI: `https://lyrics-api.boidu.dev`

Both hosts serve the same API shape.

## Endpoints

| Endpoint            | Timing        | Response                                   |
| ------------------- | ------------- | ------------------------------------------ |
| `GET /getLyrics`    | Syllable      | `{"ttml": "...", "score": 0.97}`           |
| `GET /ttml/getLyrics` | Syllable    | `{"lyrics": "...", "provider": "ttml"}`    |
| `GET /kugou/getLyrics` | Line       | `{"lyrics": "[00:12.34]...", "provider": "kugou"}` |
| `GET /qq/getLyrics` | Word          | `{"lyrics": "<QrcInfos>...", "provider": "qq"}` |

`/getLyrics` is the default TTML endpoint. The provider-specific endpoints
(`/ttml`, `/kugou`, `/qq`) return the same `lyrics` + `provider` envelope. The
CLI uses `/ttml/getLyrics` for word-level requests and `/kugou/getLyrics` for
line/plain requests.

## Query parameters

| Parameter | Aliases                    | Description                         |
| --------- | -------------------------- | ----------------------------------- |
| `s`       | `song`, `songName`         | Song title                          |
| `a`       | `artist`, `artistName`     | Artist name                         |
| `al`      | `album`, `albumName`       | Album name; improves matching       |
| `d`       | `duration`                 | Duration in whole seconds; improves matching |

Send at most one alias per field: aliases are concatenated when more than one is
sent. The request is rejected with `422` only when both song and artist are
missing. Sending album and duration is strongly recommended because they are
part of the cache key; omitting them often turns a cache hit into an uncached
request that needs an API key.

## Authentication

The API is cache-first. Cached lyrics are public; a fresh fetch may require an
API key, depending on the deployment:

```http
X-API-Key: your-api-key
```

The documentation notes that new API keys are not currently being issued
(retrieved 2026-10-24); without a key, an uncached lookup fails with `401` until
the track is cached.

Per the documentation, `/getLyrics` and `/ttml/getLyrics` require a key on cache
misses when `API_KEY_REQUIRED` is enabled. `/kugou/getLyrics` and
`/qq/getLyrics` are documented as never requiring one, but the `boidu.dev`
deployment was observed returning `401` for an uncached `/kugou` request, so key
requirements are deployment-specific.

Response headers report state: `X-Auth-Mode` on `/getLyrics` and
`/ttml/getLyrics` (`authenticated`/`invalid`/`cache`), `X-Cache-Status` on every
lyrics endpoint (`HIT`/`MISS`/`NEGATIVE_HIT`/`STALE`), and `X-Provider` on the
provider-specific endpoints.

## Rate limiting

Limits are per IP and use two tiers: a normal tier that can fetch fresh lyrics,
then a cached-only tier. Both tiers exhausted returns `429`. Limits are not part
of the contract; read `X-RateLimit-Limit`, `X-RateLimit-Remaining`,
`X-RateLimit-Type`, and `Retry-After` instead. A valid API key bypasses rate
limiting and sets `X-RateLimit-Bypass: true`.

## Errors

| Status | Meaning |
| ------ | ------- |
| 401    | Uncached request without a valid API key |
| 404    | No lyrics for this track (negative-cached for seven days) |
| 422    | Both song and artist missing (plain-text body) |
| 429    | Rate limited; honor `Retry-After` (`1` when both tiers are exhausted, `60` when only the cached tier remains and the query is not cached) |
| 500    | Upstream failure with no cached fallback |
| 503    | Cache-only mode and the query is not cached |

JSON errors carry `error` and usually `message`:

```json
{"error": "API key required", "message": "Uncached queries require a valid API key via X-API-Key header"}
```

## TTML notes

TTML timing uses `<p>` for lines and `<span>` for syllables/words. Whitespace
between spans is significant and represents spacing between words; do not
pretty-print or reformat the document before parsing. Times may be
`HH:MM:SS.mmm` or `M:SS.mmm`. The `score` field is a 0-1 match confidence sent
only on fresh fetches, not on cache hits.

## Self-hosting

Yes. GPL-3.0 licensed.
