# LogPaste on VibeNest

**NOT_READY.** This is a proposed guided pilot for a small team sharing synthetic diagnostic logs. The release source and container metadata were reviewed statically. VibeNest deployment and persistence still need verification.

## Deployment link draft

The link opens a repository preview. In the single-service flow, Continue can dispatch a first deployment after sign-in, before project settings can be edited. Wait for the protected branch to be published and for a test slot to be assigned before using Continue. The branch in this example is currently local.

```html
<a href="https://vibenest.net/deploy?repo=https%3A%2F%2Fgithub.com%2FNikitaBabenko%2Flogpaste&amp;branch=codex%2Fvibenest-logpaste-pilot">
  <img src="https://vibenest.net/deploy-to-vibenest.svg" alt="Deploy to VibeNest" width="206" height="40">
</a>
```

## Project settings for the proposed pilot

1. Use a single service from the root Dockerfile. Route HTTPS to internal port `3001` and set runtime `PORT=3001`. The upstream Dockerfile does not declare `EXPOSE`. Confirm the effective branch, revision and port in the project.
2. After the protected profile exists, set its required runtime-only credentials and canonical HTTPS origin. The proposed variable names are `LP_PILOT_USERNAME`, `LP_PILOT_PASSWORD` and `LP_PILOT_PUBLIC_ORIGIN`. Use a generated password in the platform's secret environment settings. Leave `LP_BEHIND_PROXY`, `DB_REPLICA_URL` and `LITESTREAM_*` unset. SQLite uses `/app/data/store.db` and needs neither S3 nor Litestream.
3. In the service's Storage tab, attach a dedicated persistent volume at `/app/data`. The Storage action can trigger a redeploy. The proposed profile must refuse to open its listener until its required settings are valid and the volume is mounted. For a reused project, verify its saved branch, stale environment settings and existing mount before taking an action. Use a new test volume for the pilot.
4. Once the native deployment is admitted, sign in over HTTPS, submit a synthetic log from the browser or CLI, and open the returned link. Confirm the content and that an unauthenticated request cannot read or write a paste. Redeploy through VibeNest and check the same link on the same volume. Keep the acceptance data synthetic and remove the dedicated test volume at pilot cleanup.

## Protected profile required before a launch

The upstream application accepts unauthenticated writes and serves paste text by ID. Its size flag applies to each paste and the database has no retention or aggregate limit. The release image has the same application defaults. This plan requires a small source change.

- Require authentication for browser and CLI reads and writes. Validate the required settings before listening. An optional unauthenticated health endpoint should return a fixed response with no paste or configuration data.
- Limit payloads to 64 KiB and use a global upload limit, bounded concurrent work and HTTP timeouts. Generate returned links from a configured HTTPS origin. Apply no-store responses for pastes and omit paste IDs, query strings, credentials and bodies from app access logs. Check platform access logging during acceptance.
- Expire reads at 24 hours and purge expired rows at startup and periodically. Bound physical database and journal growth within a 32 MiB pilot budget, including reused databases. Exercise admission, expiry and the storage bound before publishing the branch. Purging here means logical deletion. Remove the dedicated volume at test cleanup.
- Run the native Free-tier acceptance on the exact reviewed revision with the 256 MB, 0.5 vCPU and 4 GB platform allocation verified separately from the application storage budget.
