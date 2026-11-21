# Feature catalogue

Routes and rules of each API capability, grouped by area. The route registry
(`internal/api/routes.go`) and the generated OpenAPI remain authoritative. The
web app integrates all of them; its remaining limitations are listed in
[`app/docs/known-gaps.md`](../../app/docs/known-gaps.md).

## Deployment

Apply migrations before starting the new API. SQLC now reads the migration directory.
JSONB values serialize as JSON objects/arrays rather than base64 strings.
Binary media is persisted in PostgreSQL, with a 10 MiB per-file limit and a serialized
100 MiB daily quota per owner. Upload initiation expires after one hour.

Configure `SMTP_ADDRESS` (host:port), `SMTP_FROM`, `SMTP_USERNAME`, `SMTP_PASSWORD`,
`PASSWORD_RESET_URL`, and `PUBLIC_API_URL` in the server environment. SMTP delivery
requires STARTTLS. Compose forwards these variables. Email endpoints return 503
when delivery is unavailable; reset tokens, signing codes and surrender handover
codes are never returned in their HTTP responses. Integration tests use a mailbox
in memory instead of sending external email.

Terms/privacy are seeded with the user-requested lorem ipsum text, version
`0.1-placeholder`, explicitly marked `placeholder: true`. Administrators can publish
immutable reviewed replacement versions with an effective date. Signup records
the exact accepted versions, and accepts optional `terms_document_id` and
`privacy_document_id` to reject stale acceptance. Existing users can read and add
legal-version acceptances.

## Account, profiles, and verification

- `POST /v1/auth/forgot-password`: `{email}`; generic 202 response, throttled
  per account, 30-minute single-use recovery token delivered through email.
- `POST /v1/auth/reset-password`: `{token,password}`; changes credentials,
  revokes refresh sessions, and increments the account authentication version.
  Both access tokens and refresh sessions carry the version, preventing old
  credentials or concurrent refresh operations from restoring revoked access.
- `DELETE /v1/me`: `{password}`; deactivates the account, revokes its sessions,
  reset tokens and memberships. Adoption, welfare, financial and audit records
  are retained. A sole organization manager must transfer management first.
- `PATCH /v1/me` / `GET /v1/me`: `profile` contains CPF with checksum validation,
  birth date, occupation, biography, social links, address, housing, preferences,
  documents, avatar URL and `allow_organization_contact`. Verification status is
  server-owned; changing the profile invalidates its prior verification.
- `PATCH /v1/organizations/{id}`: chosen slug, public `profile` (type, logo,
  gallery, capacity, service areas, adoption policy) and member-only
  `private_profile` (representative identity/contact, documents, review/contact
  consent). Changing private details returns verification to pending.
- `GET /v1/me/verification/requirements` and
  `GET /v1/organizations/{id}/verification-requirements`: server requirements.
- `GET|POST /v1/me/verification` and
  `GET|POST /v1/organizations/{id}/verification-submissions`: evidence and history.
- `GET /v1/verifications`, `GET /v1/verifications/{id}`, and
  `POST /v1/verifications/{id}/decision`: administrator queue, captured identity
  snapshot and review decisions (`under_review`, `approved`, `rejected`). A changed
  identity snapshot cannot be approved using stale evidence.

## Animals and applications

- Animal create/edit/discovery accept bird, rabbit, reptile and small_mammal in
  addition to dog, cat and other. `attributes` holds height_cm, weight_kg,
  temperament, vaccination/neutering, special needs, medical conditions and energy.
- `PUT /v1/animals/{id}/organization`: `{organization_id,reason}`. Owner/admin
  assignment of pending private submissions, before any mediated case exists.
- `POST /v1/animals/{id}/archive` / `DELETE /v1/animals/{id}`: `{reason}`;
  ownership or publication permission, serialized active-application checks,
  retained records and lifecycle history. Archived animals appear in inventories.
- `GET /v1/animals/{id}/history`: lifecycle states and organization assignments,
  available to the owner and authorized current organization members/admins.
- `POST /v1/applications/{id}/cancellation`: `{reason}`; authorized organization
  management/admin cancellation with history, conversation closure, appointment
  cancellation, and release of the animal when its approved application ends.

## Media, posts, following, notifications, and chat

- `POST /v1/media`: `{filename,content_type,byte_size,purpose}` returns an upload
  path and media URL. `PUT /v1/media/{id}/content` accepts exact binary bytes,
  validates detected MIME and size, and prevents replacement after upload.
  `GET /v1/media/{id}` checks ownership, authorized case review or a public,
  approved association. Documents and private follow-up/surrender evidence stay
  private. Supported types: JPEG, PNG, WebP, PDF (documents/surrender only).
- `POST /v1/organizations/{id}/posts`, `GET /v1/posts`,
  `GET|PUT|DELETE /v1/posts/{id}`: organization posts with ordered media_ids and
  tags. Publishing requires permission and verified organization status.
- `PUT|DELETE /v1/posts/{id}/like`: idempotent per-user post likes.
  `POST /v1/reports` accepts `subject_type: post`.
- `PUT|DELETE /v1/organizations/{id}/follow`,
  `GET /v1/organizations/{id}/followers`, `GET /v1/me/following`,
  `GET /v1/feed`: follow state, counts, paginated organizations and followed posts.
- `GET /v1/notifications`, `GET /v1/notifications/unread`,
  `PUT /v1/notifications/read`, `PUT /v1/notifications/{id}/read`:
  actual persisted events and actor-scoped read actions. Database triggers notify
  participants about messages, applications, appointments, posts, donations,
  verification decisions, follow-ups and surrender cases. Due follow-up reminders
  are materialized idempotently when the recipient loads notifications/counts.
- `GET|PUT /v1/conversations/{id}/read-state`: PUT takes `{message_id}` and advances
  the user's cursor without regression; listing messages never marks them read.
  `GET /v1/conversations/unread` returns accessible conversations and unread totals.
- `POST /v1/animals/{id}/conversations`: `{user_id}`; authorized organization
  management may contact a person only when they like that animal and explicitly
  consent to organization contact. Removing the preference or consent closes the
  interested-person conversation. Access/version revocation is checked on live
  WebSocket sends and before delivering events.
- `GET /v1/animals/{id}/interest`: authorized totals and paginated consenting names.
  `GET /v1/organizations/{id}/analytics`: status totals, likes/applications and
  ninety daily aggregates. Preference events preserve history after unlike.

## Direct-transfer donations

- `GET|PUT /v1/organizations/{id}/payment-details`: published Pix/bank-transfer
  instructions; only organization managers/admins can edit them.
- `POST /v1/organizations/{id}/donations`: report amount_cents, transferred_at,
  optional private receipt_media_id, anonymity and note. This records a reported
  direct transfer; confirmation requires an organization decision.
- `GET /v1/donations` and `GET /v1/donations/{id}`: donor's records, or authorized
  organization records with `organization_id`; identities of anonymous donors are
  omitted from organization projections.
- `POST /v1/donations/{id}/decision`: `{status: confirmed|not_found,reason}`;
  transactional decision/history. `GET /v1/organizations/{id}/donation-totals`
  separates reported, confirmed and not-found BRL amounts.

## Agreements, follow-ups, surrender/return, and legal documents

- `GET|POST /v1/organizations/{id}/agreement-templates`: immutable numbered
  templates, kind adoption/surrender. Supported placeholders:
  `{{user_name}}`, `{{organization_name}}`, `{{animal_name}}`,
  `{{representative_name}}`, `{{date}}`.
- `GET|POST /v1/applications/{id}/agreement`: populate using
  `{template_id,representative_id}`. `GET /v1/agreements/{id}` returns immutable
  text, SHA256 and signatures. `POST .../signing-code` emails a ten-minute code;
  `POST .../signatures` requires `{code,sha256,accept:true}`. Codes have a five-attempt
  limit, one-minute resend throttle, and single-use consumption. Signatures retain
  signer/time/hash/IP/user-agent evidence. `GET .../print` is authorized escaped HTML.
- `GET|POST /v1/applications/{id}/follow-ups`, `GET /v1/follow-ups/{id}`:
  schedule/report history for completed adoptions. `POST .../reports` accepts notes,
  welfare_status and private media_ids. `POST .../review` requests changes or closes
  a submitted report; immutable versions and reviews are retained.
- `POST|GET /v1/surrenders`, `GET|PUT /v1/surrenders/{id}`: intake and return
  requests, participant/organization lists, evidence, resubmission and histories.
  Returns require the current completed adopter and original responsible organization.
- `POST /v1/surrenders/{id}/decision`, `POST .../cancellation`: authorized review
  and pre-handover cancellation. `GET|POST .../agreement` uses the shared signing
  workflow. `POST .../handover-code` emails the requester a code; `POST .../handover`
  requires requester delivery and authorized organization receipt. Both signatures
  and confirmations are required before intake creates/publishes an animal or a
  return makes the original animal available again. Histories remain intact.
- `GET /v1/legal/terms`, `GET /v1/legal/privacy`, corresponding `/versions` paths,
  `GET /v1/legal/documents/{id}`: effective documents and immutable version history.
  `POST /v1/legal/documents` is administrator-only publication.
  `GET|POST /v1/me/legal-acceptances` records exact accepted versions.

## Earlier implemented operations

Membership discovery, personal/organization animal inventories, preference clearing,
image moderation, adopter withdrawal, application conversation references, chat
message cursors, and appointment/confirmation lookup remain supported.

## Authorization hardening

- `POST /v1/conversations/{id}/ws-ticket`: conversation-scoped, one-minute
  WebSocket ticket for cookie-based browsers (separate JWT audience).
- `PUT /v1/me/organizations/{id}/membership`: `{decision: accepted|declined}`;
  only the invited person can respond. Declined/removed people can be re-invited.
- Reports are visible only to their reporter and administrators; a reported
  organization can no longer read or dismiss reports about itself.
- Once an organization accepts a private submission (or created the animal),
  only its active members may edit, archive or delete it.
- Unverified or suspended organizations cannot publish animals, approve
  applications, schedule appointments, receive new applications, or expose
  animal detail publicly. New evidence or profile edits do not lift a suspension.
- Renaming a verified organization, or changing its PIX key once donation
  details exist (clearing it included), returns it to `pending`. First-time
  donation setup does not. The verification snapshot includes the display name
  and PIX key, so evidence reviewed before such a change cannot be approved.
- Personal verification status is projected as `adopter_verification_status`
  in organization application views. It is a badge only and gates nothing.
- Media downloads are cacheable: `public, max-age=86400, immutable` for
  publicly visible files, `private, ...` otherwise (bytes never change).
- Follow-up reports are written only by the adopter.
- A refresh token presented more than 30 seconds after rotation revokes every
  session of the account and bumps its authentication version.

## Collections, names and workflow follow-ups

- `page.total` is the number of matching items across all pages (count queries
  mirror each list's filters; message history counts all visible messages).
- `postResponse` carries `organization_name`, `organization_slug` and
  `organization_logo_url`; `donationResponse` carries `organization_name`;
  `surrenderResponse` carries `organization_name`, `requester_name`, and each
  history record an `actor_name`. New surrender history notes are Portuguese.
- `POST /v1/applications/{id}/agreement` also accepts `completed` applications.
  Agreement templates (list and publish) require `can_manage_applications`.
- `PATCH /v1/follow-ups/{id}` `{due_at?, instructions?}` while scheduled or
  awaiting changes; `POST /v1/follow-ups/{id}/cancellation` `{reason}` for any
  open task. Status `cancelled` stores `cancellation_reason` (migration 11).
- Notifications include `application_id` (application, appointment and
  follow-up kinds) and `audience` (`requester` or `organization`). Advancing a
  conversation read cursor marks its message notifications read.

## Validation

`make check` covers generation, race tests, lint and OpenAPI validation. `make test-e2e`
uses only the isolated petfinder_e2e database. Added integration coverage exercises
recovery and session revocation, identity-review conflicts, private/public profile
projections, actual binary media, inventories and lifecycle conflicts, posts/follows,
notification/read cursors, contact consent, analytics, direct-transfer confirmation,
legal acceptance versions, agreement signatures/code limits, follow-up resubmission,
and full signed intake/return handovers.
