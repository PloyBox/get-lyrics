# lrclib API

Public lyrics API at <https://lrclib.net>. No API key is required, and the API is
CORS-enabled.

Reference: <https://lrclib.net/docs> (retrieved 2026-10-24).

## Endpoints

### `GET /api/get`

Returns the best-matching lyrics record for a track signature.

| Parameter    | Required | Type   | Description                                      |
| ------------ | -------- | ------ | ------------------------------------------------ |
| `track_name` | yes      | string | Track title                                      |
| `artist_name`| yes      | string | Artist name                                      |
| `album_name` | no       | string | Album name; improves matching                    |
| `duration`   | no       | number | Track duration in seconds, 1-3600; matches ±2 s  |

A successful response is a single lyrics record (`200`). No match returns `404`
with `TrackNotFound`.

### `GET /api/search`

Searches records by free text or structured fields. At least one of `q` or
`track_name` is required; with neither, the API returns an empty array. The
endpoint returns at most 20 results and has no pagination.

| Parameter    | Required    | Type   | Description                                                              |
| ------------ | ----------- | ------ | ------------------------------------------------------------------------ |
| `q`          | conditional | string | Free text across title, artist and album; takes precedence over other fields |
| `track_name` | conditional | string | Search in track titles                                                   |
| `artist_name`| no          | string | Search in artist names                                                   |
| `album_name` | no          | string | Search in album names                                                    |

The response is a JSON array of lyrics records, possibly empty.

### `GET /api/get/{id}`

Returns one record by its stable lrclib ID. The CLI does not use this endpoint.

## Lyrics record

All lyrics endpoints return the same object shape:

| Field          | Type           | Description                                                    |
| -------------- | -------------- | -------------------------------------------------------------- |
| `id`           | number         | Stable record ID                                               |
| `name`         | string         | Alias of `trackName`, kept for compatibility                   |
| `trackName`    | string         | Track title                                                    |
| `artistName`   | string         | Artist name                                                    |
| `albumName`    | string         | Album name                                                     |
| `duration`     | number         | Track duration in seconds                                      |
| `instrumental` | boolean        | `true` when the track has no lyrics                            |
| `hasWordSync`  | boolean        | `true` when `lyricsfile` contains word-level timings           |
| `plainLyrics`  | string \| null | Plain, unsynced lyrics; `null` for instrumental records        |
| `syncedLyrics` | string \| null | LRC-formatted lyrics with line timestamps; `null` for instrumental records |
| `lyricsfile`   | string         | Raw Lyricsfile YAML document (always present)                  |

## Errors

| Status | `name`           | Meaning                                               |
| ------ | ---------------- | ----------------------------------------------------- |
| 400    | `ValidationError`| Invalid parameter, e.g. duration outside 1-3600       |
| 401    | `InvalidAppIdError` / `MissingApiKeyError` / `InvalidApiKeyError` | Invalid optional application credentials |
| 404    | `TrackNotFound`  | No matching record                                    |
| 429    | `RateLimitExceeded` | Rate limit exceeded; honor `Retry-After`           |
| 500    | `UnknownError`   | Unexpected server error                               |
| 503    | `ServerOverloaded` | Temporarily busy; retry after `Retry-After`         |

Malformed or missing query parameters can be rejected by the framework with a
plain-text body, so check the status code and `Content-Type` before decoding.

## Client requirements

Clients must identify themselves with a `User-Agent` header containing the
application name, version and a project URL or email. `X-User-Agent` and
`Lrclib-Client` are accepted alternatives where `User-Agent` cannot be set.

Rate limits are approximate and can change:

| Client identification                                  | Approximate allowance          |
| ------------------------------------------------------ | ------------------------------ |
| Generic client (no/unknown `User-Agent`)               | ~45 requests/min per IP        |
| Identified client (`User-Agent` with app name)         | ~90 requests/min per IP        |
| Registered client-side app (`Lrclib-App-Id`)           | ~180 requests/min per IP, plus app-wide allowance |
| Registered server-side app (`Lrclib-Api-Key`)          | App-wide allowance (~6,000 requests/min) |

Send requests sequentially and add a short delay between batch calls. A `429`
response includes `Retry-After`; wait at least that long, with jitter, before
retrying.

## Self-hosting

Yes. MIT licensed.
