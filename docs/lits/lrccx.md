# lrc.cx API

Public lyrics API from the LrcAPI project. The legacy endpoint is served at
<https://api.lrc.cx>; the CLI uses its batch-search API.

Reference: <https://docs.lrc.cx/docs/legacy/lyrics/> (retrieved 2026-10-28).
Chinese-language documentation.

## Endpoints

### `GET /jsonapi`

Returns multiple matches as JSON, ranked by match score in descending order.

| Parameter | Required | Type   | Description                                                   |
| --------- | -------- | ------ | ------------------------------------------------------------- |
| `title`   | yes      | string | Song title                                                    |
| `artist`  | no       | string | Artist name; supports multiple separators                     |
| `album`   | no       | string | Album name; the literal `[Unknown Album]` is treated as empty |
| `path`    | no       | string | Local music-file path, used to read `.lrc` files and tags     |

The `path` parameter does not apply to the CLI and is never sent.

### `GET /lyrics`

Returns the single best match as raw LRC text. The documentation lists
`text/html`; live responses use `text/plain`. The CLI does not use this
endpoint.

## Response

A `200` response from `/jsonapi` is a JSON array; an empty or missing result may
instead produce `404` with `{"message": "未找到歌词"}` ("lyrics not found").

The service is a community aggregator over several upstreams. Each entry
mirrors the upstream it came from, so the field set varies. Shapes below were
observed in live responses on 2026-10-28.

Fields seen on every entry:

| Field         | Type           | Description                                         |
| ------------- | -------------- | --------------------------------------------------- |
| `id`          | string         | Entry ID; semantics differ per upstream, see below  |
| `title`       | string         | Track title                                         |
| `artist`      | string         | Artist name                                         |
| `album`       | string \| null | Album name; `null` when unknown                     |
| `lrc`         | string \| null | LRC lyrics text; `null` when the entry carries none |
| `score`       | number         | Match score; entries are sorted descending          |
| `cover`       | string         | Cover image URL; may be empty                       |
| `create_time` | string         | ISO-8601 creation time                              |

Non-Apple entries additionally carry:

| Field       | Type   | Description                                 |
| ----------- | ------ | ------------------------------------------- |
| `from`      | string | Upstream provider, e.g. `Netease`; optional |
| `hash`      | string | Content hash                                |
| `timestamp` | number | Record timestamp                            |

The upstream documentation describes `id` on these entries as a hash of the LRC
text. Observed forms are 32-char hex (equal to `hash`) and a 20-char
base64-style token. Sampled tokens share a common segment, which suggests
internal record IDs rather than content hashes.

Apple Music entries additionally carry:

| Field          | Type   | Description                                                                                              |
| -------------- | ------ | -------------------------------------------------------------------------------------------------------- |
| `lrc_ttml`     | string | Apple Music TTML document; timing may be syllable, line or none; empty when the track has no TTML lyrics |
| `isrc`         | string | Track ISRC                                                                                               |
| `object_id`    | string | Apple Music catalog track ID; equal to `id`                                                              |
| `lyric_path`   | string | Apple Music API path the TTML was fetched from                                                           |
| `duration`     | number | Track duration in seconds                                                                                |
| `releaseDate`  | string | Release date                                                                                             |
| `trackNumber`  | number | Track number on the album                                                                                |
| `discNumber`   | number | Disc number                                                                                              |
| `composerName` | string | Composer names                                                                                           |
| `genreNames`   | array  | Genre names                                                                                              |
| `audioLocale`  | string | Content language locale (e.g. `en-US`, `zxx`)                                                            |
| `object_type`  | string | Object type, `song`                                                                                      |
| `update_time`  | number | Record update time (Unix seconds)                                                                        |
| `cover_format` | string | Cover URL template with `{w}`/`{h}` size placeholders                                                    |
| `artists_id`   | array  | Artist IDs                                                                                               |
| `album_id`     | array  | Album ID                                                                                                 |

On these entries `id` is the Apple Music catalog track ID (equal to
`object_id`), not an LRC hash.

Their `lrc` text mirrors the `lrc_ttml` content.

### Documentation discrepancy

The legacy documentation lists the lyrics text under a `lyrics` field. The live
`/jsonapi` response uses `lrc` instead, and the CLI reads `lrc`. Treat the
documentation field name as outdated.

## Notes

- No authentication is documented.
- No rate limit is documented.

## Self-hosting

Yes. GPL-3.0 licensed.
