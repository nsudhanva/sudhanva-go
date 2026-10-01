# Changelog

## 0.2.0 — 2026-10-01

- Requires Go 1.26 or newer. Tested on Go 1.26 and 1.27.
- Error responses in RFC 9457 problem format, which the profile-insight endpoints return, now report their `detail` and `type` instead of a generic "request failed".
- `APIError` exposes `Hint` and `DocsURL` from the standard error envelope, and unexpected error bodies keep any string `error` as the message.
- `Profile.WorksFor` and `BatchResponse.Count` decode the published `worksFor` and `count` fields.
- Exports `sudhanva.Version`, which the `User-Agent` header reports.

## 0.1.0 — 2026-08-24

- First release: profile, articles, search, batch reads, and profile-insight jobs.
