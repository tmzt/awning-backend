package common

const (
	PRIVATE_CREDENTIALS_FILE   = ".private/service_credentials.json"
	PRIVATE_CREDENTIALS_DOTENV = ".env.private"
	DEFAULT_PROMPT_NAME        = "prompt4"
	DEFAULT_CONFIG_DIR         = ".config/"
	DEFAULT_CONFIG_FILE        = "config.json"

	DEFAULT_MIN_INPUT_TOKENS  = 1
	DEFAULT_MAX_INPUT_TOKENS  = 200000
	DEFAULT_MAX_OUTPUT_TOKENS = 400000
	DEFAULT_REDIS_ADDR        = "localhost:6379"
	DEFAULT_REDIS_PASSWORD    = ""
	DEFAULT_REDIS_PREFIX      = "awning:"
	DEFAULT_LISTEN_ADDR       = ":4000"

	DEFAULT_ENABLED_MODELS = "qwen/qwen3-next-80b-a3b-thinking-maas"
	DEFAULT_MODEL          = "qwen/qwen3-next-80b-a3b-thinking-maas"

	DEFAULT_VAR_DIR = ".var"

	// Unsplash API constants
	UNSPLASH_API_BASE_URL = "https://api.unsplash.com"
)
