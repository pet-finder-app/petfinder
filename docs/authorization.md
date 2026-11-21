# Authorization

The API is the only authority on permissions; clients only hide actions that would be refused.

## Identity and sessions

- JWT access tokens (HS256) with mandatory issuer, audience, `exp` and `iat`, valid for 15 minutes. Every protected route checks in the database that the account is active and that the token's authentication version is current.
- Refresh tokens rotate on every use and are stored only as hashes. Presenting a refresh token more than 30 seconds after it was rotated is treated as theft: every session of the account is revoked and its access tokens stop working.
- Resetting the password or deleting the account also revokes every session.
- Global roles: `adopter`, `advertiser`, `organization_member` and `platform_admin`. Administrative routes require `platform_admin`.

## Organization membership

Permissions inside an NGO come from an **active** row in `organization_members`; invitations grant nothing until the invited person accepts them (`PUT /v1/me/organizations/{id}/membership`).

| Permission                | Allows                                                                                   |
| ------------------------- | ---------------------------------------------------------------------------------------- |
| `can_manage_members`      | Team, NGO data and verification, donation details, donation decisions                    |
| `can_publish_animals`     | Publishing and editing animals and posts, image moderation, archiving animals            |
| `can_manage_applications` | Application decisions, appointments, agreements and templates, follow-ups, surrenders    |

Any active member can read the NGO's inventory, applications and conversations.

## Verification rules

- Only verified, active NGOs publish animals, appear in discovery, receive new applications, approve applications and schedule visits. A suspended NGO keeps read access to its records.
- Renaming a verified NGO, changing its private details, or changing its PIX key (after the first setup) returns it to `pending`. Submitting new evidence never lifts a suspension; only an administrator's decision does.
- Personal verification only shows a badge; it neither grants nor blocks actions. Changing the profile invalidates the previous verification.
- Evidence keeps a snapshot of the verified data; if the data changes during review, approval is refused.

## Per-resource rules

- **Animals:** whoever submitted a private animal can edit it only until the NGO accepts it; after that, only active members of the responsible NGO.
- **Reports:** visible only to the reporter and administrators; the reported organization can neither read nor close them.
- **Conversations and WebSocket:** the interested person and active members of the NGO; every send is authorized again.
- **Follow-ups:** only the adopter submits reports; the NGO reviews, reschedules or cancels.
- **Media:** see [database.md](database.md#media).
- **Anonymous donations:** the donor's identity is hidden from the NGO (an attached receipt remains visible to it).

The end-to-end tests in `e2e/` cover these cases, including denied access.
