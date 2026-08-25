# Router (Build.io deploy context)

Terms for the Build.io + Supabase deployment path of this router fork.

## Language

**Router app**:
The Build.io application named `router` on the fenil team (dockerfile stack, ap-northeast-1). The deploy target for this effort.
_Avoid_: aiand-relay (legacy app), dyno name alone

**Supabase DATABASE_URL**:
The manually set Build.io config var pointing at Supabase Postgres (session pooler for runtime). Not an Addons.io provisioned URL.
_Avoid_: Schema To Go, Ave To Go, dedicated Build.io database addon

**Host WSL path**:
Local `make setup` / `make dev` against the same Supabase session pooler (`:5432`), with `PUBSUB_DISABLED=true` and no Compose Postgres. Documented in `docs/HOST_WSL_SUPABASE.md`. Skip `make db` and `make full-setup`.
_Avoid_: Treating Compose `make db` as required for every local loop

**Pub/Sub disable**:
Single-replica boot with `PUBSUB_DISABLED=true` (or empty `PUBSUB_PROJECT_ID`) so GCP Pub/Sub is skipped. Cross-replica cache invalidation is off; 5-minute cache TTL is the safety net.
_Avoid_: "no Pub/Sub features", emulator-only disable

**Pub/Sub re-enable**:
Turning Pub/Sub back on only when project ID, topic, subscription prefix, and GCP credentials are all present together, with `PUBSUB_DISABLED` unset/false.
_Avoid_: Setting `PUBSUB_PROJECT_ID` alone

**Deploy dump**:
Extra or conflicting Build.io deploy artifacts that confuse the operator (duplicate guides, wrong addon in `app.json`, compose/emulator leftovers treated as production).
_Avoid_: Legitimate local compose for development
