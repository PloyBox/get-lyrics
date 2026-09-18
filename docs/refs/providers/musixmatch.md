# musixmatch

Backed by the Musixmatch API.

Every request carries the API key as the `apikey` query parameter, taken from the required
custom `--env` key `MUSIXMATCH_API_KEY`. The lookup path is chosen by the first matching
`Request` shape:

```
track.get → track.lyrics.get / track.subtitle.get      when ISRC is set
matcher.lyrics.get / matcher.subtitle.get              when Song + Author are set
track.search → track.lyrics.get / track.subtitle.get   when only Song is set
```

- An ISRC identifies the track precisely, so it wins over the author: `track.get` is keyed by
  the ISRC alone and takes no artist, so with an ISRC set the author filter is dropped (the
  fetch layer warns it is ignored).
- The matcher endpoints are fuzzy lookups keyed by title + artist; with no artist (and no
  ISRC) the API cannot match, so a title-only request searches for the track and fetches
  lyrics/subtitles by commontrack id.
- Subtitle endpoints require the paid Scale plan; on cheaper plans they return 402/403, which
  the adapter treats as "no synced lyrics" and falls back to the plain track (the fetch layer
  reports the downgrade). A synced request returns exactly one lyrics track — the subtitle
  when available, else the plain lyrics.
- Album is not supported — neither the matcher nor the search endpoints take an album
  parameter.
- `do` issues one GET against a method, injecting the API key, and decodes the JSON envelope.
  The effective status is `message.header.status_code` when present — Musixmatch returns HTTP
  200 with an error code in the body for many failures — falling back to the HTTP status.
- `decodeResponse` maps the envelope: an empty/`[]`/`{}` body on success is the API's
  no-match signal (`errNotFound`); 401/402/403 get explicit, actionable messages; 404 → not
  found; anything else uses the API hint or a bounded body excerpt.
- `getTrackByISRC` treats an empty commontrack id in the response as no match.
- `searchTrack` finds the first search hit with `has_lyrics` and a commontrack id, ranked by
  track rating (`s_track_rating=desc`) so the best-known match comes first.
- `cleanLyrics` trims the appended `*******` usage notice (API boilerplate, not lyrics) and
  treats the instrumental placeholder `....` as empty.
- 10s timeout.
