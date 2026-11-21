# Database

Migrations live in `db/migrations`, queries in `db/queries` and generated code in `internal/database`. To change the database:

```sh
make migrate-create NAME=change_name
# edit the generated .up.sql and .down.sql files
make migrate-up
make generate
make test
```

Never edit `internal/database` by hand: change the SQL and regenerate the package. Each paginated list query has a matching count query in `db/queries/counts.sql` with the same filters; keep them in sync.

## Media

Uploaded files are stored in PostgreSQL itself, in the `content` (`bytea`) column of `media_uploads`. Uploading takes two steps: `POST /v1/media` records name, type, size and purpose; `PUT /v1/media/{id}/content` receives the exact bytes, checks the size and detected type (JPEG, PNG, WebP; PDF only for documents and surrenders) and never allows replacement. Limits: 10 MiB per file and 100 MiB per user per day; an upload expires after one hour.

`GET /v1/media/{id}` serves a file only to its owner, administrators, the parties of a case that references it, or publicly when it belongs to approved public content (animal photos and posts of verified NGOs, a verified NGO's logo and gallery, profile photos). Since bytes never change, responses are cacheable for one day (`public` for public content, `private` otherwise).
