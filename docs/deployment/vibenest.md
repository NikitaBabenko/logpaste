# LogPaste on VibeNest

**NOT_READY.** This guided pilot is for small teams sharing synthetic diagnostic logs. The protected profile is in a local branch of [the LogPaste fork](https://github.com/NikitaBabenko/logpaste). VibeNest launch, HTTPS access and persistence still need runtime verification.

## Deployment link draft

The link opens the repository preview. In the single-service flow, Continue can dispatch a first attempt after sign-in, before project settings can be edited. Wait for the protected branch to be published and for a test slot to be assigned before using Continue.

```html
<a href="https://vibenest.net/deploy?repo=https%3A%2F%2Fgithub.com%2FNikitaBabenko%2Flogpaste&amp;branch=codex%2Fvibenest-logpaste-pilot">
  <img src="https://vibenest.net/deploy-to-vibenest.svg" alt="Deploy to VibeNest" width="206" height="40">
</a>
```

## Project settings

1. Select the root Dockerfile and its final `vibenest` stage. Confirm the protected branch and revision. The Dockerfile also keeps an explicit `upstream` target for the original mode. Route the branded HTTPS host to internal port `3001`. If setting `PORT` explicitly, use `3001`. The image health check uses `GET /healthz`, which returns an empty response.
2. Set runtime `LP_PILOT_USERNAME`, secret `LP_PILOT_PASSWORD` with a generated password of at least 24 characters, and `LP_PILOT_PUBLIC_ORIGIN` to the exact lowercase HTTPS origin with no path, port or trailing slash. Clear `DB_REPLICA_URL` on reused projects. The pilot does not start Litestream or use forwarded IP headers.
3. In Storage, attach a dedicated writable persistent volume at `/app/data`. That action can dispatch a redeploy. The service exits before listening when settings or mount checks fail. On a reused project, confirm the saved branch, environment and volume identity first. Start with an empty volume or reuse this pilot's own compatible volume. If the app reports incompatible data, preserve the volume and resolve the mismatch before retrying. An interruption after exclusive file creation and before SQLite commits the database identity can leave an empty or uninitialized `store.db`. The next start refuses that file and preserves it for inspection.
4. After the native deploy is admitted, open the HTTPS site and use the browser's authentication prompt. Upload a synthetic log and open the returned link. For CLI uploads, pass your username to curl so it prompts for the password.

```sh
curl --user "$LP_PILOT_USERNAME" --form '_=<fixture.log' "$LP_PILOT_PUBLIC_ORIGIN/"
curl --user "$LP_PILOT_USERNAME" --request PUT --data-binary @fixture.log "$LP_PILOT_PUBLIC_ORIGIN/"
```

The multipart response contains the HTTPS link. The PUT response contains a paste ID. Open the ID under the same origin with authentication. Keep the password out of command arguments and logs.

## Limits and acceptance

- The service authenticates the UI and all paste routes. It accepts up to 64 KiB of paste data, with 1 KiB of framing allowance for multipart requests. An instance accepts a burst of two authenticated uploads, refills at six per minute and runs up to four authenticated requests at once. Paste responses use `Cache-Control: no-store`. The app omits request access logs. Check proxy and provider log behavior during acceptance.
- Pastes stop being readable after 24 hours. The service logically deletes expired rows at startup and every 15 minutes. SQLite may retain bytes in freed pages for reuse. This is logical retention and does not erase the volume. Preserve the volume across the redeploy used to check persistence. Handle any later volume removal as a separate, explicit cleanup action.
- The pilot uses `/app/data/store.db`. The main file is limited to 2,048 pages of 4 KiB on one pinned connection. The pilot uses a DELETE rollback journal and disables cache spill. The 32 MiB combined figure is a conservative test budget with journal headroom. It is not a filesystem quota. Measure the main file and journal under saturation, expiry cleanup and an interrupted write. The mount check identifies a separate writable mount and rejects common memory-backed mounts. Verify the actual persistent volume identity through VibeNest.
- In the assigned Free slot, record the exact gated attempt and applied 256 MB RAM, 0.5 vCPU and 4 GB platform allocation. Verify the branded HTTPS origin and that HTTP redirects before credentials or data are sent. Test browser upload, CLI upload and the same link across a native redeploy. Measure startup, idle and bounded-load RAM, CPU and disk.
- Check startup with missing settings, a wrong or read-only mount and incompatible pre-existing data. Test anonymous and wrong-password requests, cross-origin writes, oversized PUT and multipart bodies, rate and concurrent request limits, expired reads, storage saturation and reuse after restart. Incompatible-data checks should leave the original files intact. Use only synthetic fixtures.
