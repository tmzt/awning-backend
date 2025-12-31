# Awning.app Backend

Minimal backend for chat & image helpers, using Vertex AI for generation.

## Quick setup

1. Provide Vertex AI credentials (via `SERVICE_CREDENTIALS_JSON` or the file referenced by `.env.private`).
2. Configure Redis if used (see `common/config.go`).
3. Run locally:

```bash
go run main.go
```

Default listen address is configured in the app config (see [common/config.go](common/config.go)).

## API (current)

- **POST /api/v1/chat/stream** : Start a streaming chat generation (server-sent events). Body: prompt/input is read from the request body (see `handlers/chat.go`).
- **GET /api/v1/chat/:id** : Retrieve a previous chat/session by ID.
- **DELETE /api/v1/chat/:id** : Delete an existing chat/session by ID.

Image endpoints are available only when Unsplash keys are configured:
- **GET /api/v1/images/search** : Search photos. Query params typically include `query` (or `q`), `page`, `per_page`.
- **GET /api/v1/images/photos/:id** : Get photo details by Unsplash photo ID.

## Notes

- Static files can be served from the `APP_PUBLIC` directory when set.
- See `main.go` for route registration and `handlers/` for request/response shapes.

## Dependencies

- `github.com/gin-gonic/gin` for HTTP routing
- Redis client for session storage (optional)
- Vertex AI integration in `vertex_openai.go` / `vertex.go`

For implementation details, check `main.go`, `handlers/chat.go`, and `handlers/image.go`.

## Examples

Chat stream request (JSON body):

```json
{
	"chat_id": "",                    
	"chat_stage": "initial_creation",
	"message": {
		"role": "user",
		"content": "Describe a simple homepage for my landscaping business.",
		"context": {
			"onboarding_data": {
				"businessName": "Larry's Landscaping",
				"businessType": { "id": "landscaping", "label": "Landscaping", "category": "services" },
				"goals": ["serviceInfo"],
				"selectedMotif": "generic",
				"completed": false
			}
		}
	},
	"variables": { "owner": "Larry" }
}
```

Streaming events (Server-Sent Events):

- `start` event: `{ "chat_id":"<id>" }`
- `content` events: `{ "type":"content", "content":"...partial text..." }` (sent repeatedly)
- `done` event: contains the final response payload, example:

```json
{
	"type": "done",
	"response": {
		"chat_id": "<id>",
		"chat_stage": "initial_creation",
		"message": {
			"id": "<msg-id>",
			"role": "assistant",
			"content": "Full assistant message text...",
			"timestamp": 1700000000
		},
		"timestamp": 1700000000
	}
}
```

Image search request (example HTTP):

```
GET /api/v1/images/search?query=landscape&page=1&per_page=10
```

Image search response shape (abridged):

```json
{
	"total": 1234,
	"total_pages": 124,
	"results": [
		{
			"id": "abc123",
			"width": 4000,
			"height": 3000,
			"description": "A landscaped yard",
			"urls": { "raw":"...", "full":"...", "regular":"...", "small":"...", "thumb":"..." },
			"user": { "id":"u1", "username":"photographer" }
		}
	]
}
```

