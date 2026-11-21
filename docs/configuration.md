# Configuration

The API reads its configuration only from the environment. See [.env.example](../.env.example) for a complete local file.

| Variable            | Required | Default         | Description                          |
| ------------------- | -------: | --------------- | ------------------------------------ |
| `DATABASE_URL`      |      yes | —               | PostgreSQL URL                       |
| `JWT_SECRET`        |      yes | —               | Secret of at least 32 bytes          |
| `HTTP_ADDR`         |       no | `:8080`         | HTTP listen address                  |
| `APP_ENV`           |       no | `development`   | Environment name                     |
| `JWT_ISSUER`        |       no | `petfinder-api` | Access token issuer                  |
| `JWT_AUDIENCE`      |       no | `petfinder`     | Access token audience                |
| `ACCESS_TOKEN_TTL`  |       no | `15m`           | Access token lifetime                |
| `REFRESH_TOKEN_TTL` |       no | `720h`          | Maximum lifetime of a renewable session |
| `SHUTDOWN_TIMEOUT`  |       no | `10s`           | Graceful shutdown deadline           |

In production, terminate TLS at the ingress proxy, use a random secret from a vault, PostgreSQL with SSL, and credentials different from local ones.

## Transactional e-mail

Password recovery, agreement signing codes and animal handover codes are sent through an authenticated SMTP relay using STARTTLS. Without `SMTP_ADDRESS`, those routes answer `503` and the web app shows the feature as unavailable. Messages are plain text in Portuguese, the language of the product.

| Variable             | Description                                                            |
| -------------------- | ---------------------------------------------------------------------- |
| `SMTP_ADDRESS`       | Relay `host:port`; empty disables delivery                             |
| `SMTP_USERNAME`      | SMTP user                                                              |
| `SMTP_PASSWORD`      | Provider password or API key                                           |
| `SMTP_FROM`          | Sender, e.g. `Petfinder <no-reply@yourdomain.com>`                     |
| `PASSWORD_RESET_URL` | The web app's reset page; the API appends `?token=…`                   |
| `PUBLIC_API_URL`     | Public API URL, used in media links and the chat WebSocket URL         |

Example with [Resend](https://resend.com) (use port 587; port 465 requires implicit TLS, which this client does not use):

```dotenv
SMTP_ADDRESS=smtp.resend.com:587
SMTP_USERNAME=resend
SMTP_PASSWORD=re_xxxxxxxx
SMTP_FROM=Petfinder <no-reply@yourdomain.com>
```

The sender must belong to a domain verified with the provider. Integration tests use an in-memory mailbox and never send e-mail.
