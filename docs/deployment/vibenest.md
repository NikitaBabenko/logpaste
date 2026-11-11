# LogPaste on VibeNest

**NOT_READY.** This guided pilot is for small teams sharing synthetic diagnostic logs. The protected profile is published on [the pilot branch](https://github.com/NikitaBabenko/logpaste/tree/codex/vibenest-logpaste-pilot). The VibeNest build, button, HTTPS access and persistence still need runtime verification.

## Deployment link draft

The draft link opens the repository preview. With a fresh account and no matching project, **Continue** can start a single-service deployment after sign-in without pausing for configuration. The pilot button test requires an already prepared project and sign-in as its owner. Wait for an assigned test slot and complete the setup below before using **Continue**.

```html
<a href="https://vibenest.net/deploy?repo=https%3A%2F%2Fgithub.com%2FNikitaBabenko%2Flogpaste&amp;branch=codex%2Fvibenest-logpaste-pilot">
  <img src="https://vibenest.net/deploy-to-vibenest.svg" alt="Deploy to VibeNest" width="206" height="40">
</a>
```

## Prepare a solo project

1. Create a new solo project in the dashboard and leave the Git repository blank.
2. Open **Storage** and use **Attach Volume** with `/app/data` while the repository is still blank. The dashboard saves the path and calls the deployment dispatcher. With no repository configured, the request stops before reaching the provider. Check that the path is saved and shows **Unverified**. The volume is verified during the later deploy. Keep an existing pilot volume in place when reusing a project.
3. In **Environment**, add and save runtime `LP_PILOT_USERNAME`, secret `LP_PILOT_PASSWORD` with a generated password of at least 24 characters, and `LP_PILOT_PUBLIC_ORIGIN` set to the exact lowercase HTTPS origin with no path, port or trailing slash. Leave `DB_REPLICA_URL` unset. The pilot skips Litestream and ignores forwarded IP headers.
4. In **Deployment**, enter `https://github.com/NikitaBabenko/logpaste`. Under **Advanced Settings**, set the branch to `codex/vibenest-logpaste-pilot`, base directory to `/`, build pack to **Dockerfile**, and internal port to `3001`. Choose **Save Configuration**. The root Dockerfile defaults to the protected `vibenest` stage and also includes an explicit `upstream` stage for the original mode. If setting runtime `PORT` explicitly, use `3001`.
5. Attach the designated unattached Free server through **My Servers**. This saves the hardware attachment without starting a deployment. The project card's **Attach hardware** action can start a deployment for an idle project with a repository, so use **My Servers** when preparing the button test.

## Choose one launch action

After the pilot slot is assigned, confirm the intended owner, saved repository and branch, environment, storage path and Free attachment. For a manual launch, select **Deploy Now** once. For the button test, open the draft link and use **Continue** as the same owner. The solo flow selects the newest unarchived project matching that owner and repository and uses its saved branch. Check that this is the prepared project before using **Continue**. The button flow itself requests the deployment.

If deployment cannot verify the required volume, resolve that failure before retrying. The app also checks its settings and mount before listening. Use a dedicated empty volume or this pilot's own compatible volume. An interrupted initialization can leave an empty or uninitialized `store.db`. If the app refuses that file or other incompatible data, preserve the volume for inspection.

## Check upload and persistence

After the native deploy is admitted, confirm the running revision and volume identity. The image health check uses `GET /healthz`, which returns an empty response. Open the HTTPS site and use the browser's authentication prompt. Upload a synthetic log and open the returned link. For CLI uploads, pass your username to curl so it prompts for the password.

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
