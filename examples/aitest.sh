export ENDPOINT=aiplatform.googleapis.com
export REGION=global
export PROJECT_ID="sanctum-dev-479720"

curl \
  -X POST \
  -H "Authorization: Bearer $(gcloud auth print-access-token)" \
  -H "Content-Type: application/json" https://${ENDPOINT}/v1/projects/${PROJECT_ID}/locations/${REGION}/endpoints/openapi/chat/completions \
  -d '{"model":"qwen/qwen3-next-80b-a3b-thinking-maas", "stream":true, "messages":[{"role": "user", "content": "Summer travel plan to Paris"}]}'
