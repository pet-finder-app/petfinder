# Real-time chat

`GET /v1/conversations/{conversationId}/ws` opens a WebSocket with the `petfinder.chat.v1` subprotocol and applies the same access rules as the message routes. Native clients send the `Authorization` header; browsers, which cannot set headers on a WebSocket, offer the token as an extra subprotocol:

```js
const socket = new WebSocket(`wss://api.example/v1/conversations/${id}/ws`, [
  "petfinder.chat.v1",
  `bearer.${accessToken}`,
]);
socket.send(
  JSON.stringify({ type: "message.send", client_id: "local-1", body: "Olá!" }),
);
```

Web apps that keep the token in an HttpOnly cookie must not expose it to JavaScript. In that case the app's server calls `POST /v1/conversations/{conversationId}/ws-ticket` on the user's behalf and hands the browser a ticket that is valid for one minute, limited to that conversation and useless as an access token. The browser offers it as a `ticket.<ticket>` subprotocol on the returned URL (derived from `PUBLIC_API_URL`); the connection lasts until the access token that issued the ticket expires.

The server sends `message.ack` to the sender (with its `client_id` and any moderation `warning`), `message.created` to the conversation's other connections, and `error` with a problem object when a command fails. Messages sent through the REST route are broadcast as well. Every send is authorized again, so closed conversations and revoked memberships apply without reconnecting.

The connection closes with code `4001` when the access token expires: refresh the token, reconnect and list messages to fill the gap. Clients that cannot keep up are disconnected with `1008`, and server shutdown uses `1001`. Fan-out happens in memory, inside the process; running several API replicas would require a shared broker.
