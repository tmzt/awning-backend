# Sanctum Backend

A Go backend service for handling evaluations using Google Vertex AI.

## Setup

1. Set up Google Cloud credentials:
   - Create a service account with Vertex AI permissions.
   - Download the JSON key file.
   - Set the environment variable: `export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json`

2. Ensure Vertex AI API is enabled in your Google Cloud project.

3. Run the service:
   ```bash
   go run main.go
   ```

The server will start on port 8080.

## API

### POST /api/v1/evaluations

Creates a new evaluation by generating a response using Google Vertex AI.

**Request Body:**
```json
{
  "input": "Your input text here"
}
```

**Response:**
```json
{
  "id": "123456789",
  "input": "Your input text here",
  "response": "Generated response from AI",
  "timestamp": "2025-11-29T12:00:00Z"
}
```

Evaluations are stored in memory.

## Dependencies

- Gin for HTTP server
- Google Generative AI Go client for Vertex AI integration