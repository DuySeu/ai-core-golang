package providers

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Database Database `json:"database" yaml:"database"`
	Qdrant   Qdrant   `json:"qdrant" yaml:"qdrant"`
	MinIO    MinIO    `json:"minio" yaml:"minio"`
	LLM      LLM      `json:"llm" yaml:"llm"`
}

type MinIO struct {
	Endpoint  string `json:"endpoint" yaml:"endpoint"`
	AccessKey string `json:"accessKey" yaml:"accessKey"`
	SecretKey string `json:"secretKey" yaml:"secretKey"`
	Bucket    string `json:"bucket" yaml:"bucket"`
	SSLMode   bool   `json:"useSSL" yaml:"useSSL"`
}

type Database struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	Dbname   string `json:"dbname" yaml:"dbname"`
	SSLMode  string `json:"sslMode" yaml:"sslMode"`
}

type Qdrant struct {
	Host       string `json:"host" yaml:"host"`
	Port       int    `json:"port" yaml:"port"`
	APIKey     string `json:"apiKey" yaml:"apiKey"`
	Collection string `json:"collection" yaml:"collection"`
	UseHTTPS   bool   `json:"useHTTPS" yaml:"useHTTPS"`
}

type LLM struct {
	OpenAI     OpenAI     `json:"openai" yaml:"openai,omitempty"`
	Anthropic  Anthropic  `json:"anthropic" yaml:"anthropic,omitempty"`
	OpenRouter OpenRouter `json:"openrouter" yaml:"openrouter,omitempty"`
}

type OpenRouter struct {
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"baseURL" yaml:"baseURL"`
}

type OpenAI struct {
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"baseURL" yaml:"baseURL"`
}

type Anthropic struct {
	APIKey  string              `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	BaseURL string              `json:"baseURL,omitempty" yaml:"baseURL,omitempty"`
	AWS     AWSCredentialConfig `json:"aws,omitempty" yaml:"aws,omitempty"`
}

type AWSCredentialConfig struct {
	Type            string `json:"type" yaml:"type"` // "default" or "assume_role"
	Region          string `json:"region" yaml:"region"`
	RoleARN         string `json:"roleArn,omitempty" yaml:"roleArn,omitempty"`                 // required if Type is "assume_role"
	Duration        int64  `json:"duration,omitempty" yaml:"duration,omitempty"`               // in seconds, optional
	RoleSessionName string `json:"roleSessionName,omitempty" yaml:"roleSessionName,omitempty"` // optional
}

func LoadServerConfig() Config {
	return Config{
		// Database: Database{
		// 	Host:     os.Getenv("DB_HOST"),
		// 	Port:     getEnvInt("DB_PORT", 5432),
		// 	User:     os.Getenv("DB_USERNAME"),
		// 	Password: os.Getenv("DB_PASSWORD"),
		// 	Dbname:   os.Getenv("DB_DATABASE"),
		// 	SSLMode:  os.Getenv("DB_SSLMODE"),
		// },
		Qdrant: Qdrant{
			Host:       os.Getenv("QDRANT_HOST"),
			Port:       getEnvInt("QDRANT_PORT", 6334),
			APIKey:     os.Getenv("QDRANT_API_KEY"),
			Collection: os.Getenv("QDRANT_COLLECTION"),
			UseHTTPS:   os.Getenv("QDRANT_USE_HTTPS") == "true",
		},
		// MinIO: MinIO{
		// 	Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		// 	AccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		// 	SecretKey: os.Getenv("MINIO_SECRET_KEY"),
		// 	Bucket:    os.Getenv("MINIO_BUCKET"),
		// 	SSLMode:   os.Getenv("MINIO_SSLMODE") == "true",
		// },
		LLM: LLM{
			OpenAI: OpenAI{
				APIKey:  os.Getenv("OPENROUTER_API_KEY"),
				BaseURL: "https://openrouter.ai/api/v1",
			},
			Anthropic: Anthropic{
				BaseURL: "https://openrouter.ai/api",
				APIKey:  os.Getenv("OPENROUTER_API_KEY"),
			},
			OpenRouter: OpenRouter{
				APIKey:  os.Getenv("OPENROUTER_API_KEY"),
				BaseURL: "https://openrouter.ai/api/v1",
			},
			// Anthropic: Anthropic{
			// 	AuthType: "aws",
			// 	AWS: AWSCredentialConfig{
			// 		Type:            "assume_role",
			// 		Region:          "ap-southeast-1",
			// 		RoleARN:         "arn:aws:iam::130506138320:role/bedrock-cross-account-role",
			// 		Duration:        3600,
			// 		RoleSessionName: "llm-test-session",
			// 	},
			// },
		},
	}
}

func LoadLLMConfig() LLM {
	return LLM{
		OpenAI: OpenAI{
			APIKey:  os.Getenv("OPENROUTER_API_KEY"),
			BaseURL: "https://openrouter.ai/api/v1",
		},
		Anthropic: Anthropic{
			BaseURL: "https://openrouter.ai/api",
			APIKey:  os.Getenv("OPENROUTER_API_KEY"),
		},
		OpenRouter: OpenRouter{
			APIKey:  os.Getenv("OPENROUTER_API_KEY"),
			BaseURL: "https://openrouter.ai/api/v1",
		},
	}
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func (c *Config) GetDBURL() string {
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}
	return "postgres://" + c.Database.User + ":" + url.QueryEscape(c.Database.Password) + "@" + c.Database.Host + ":" + strconv.Itoa(c.Database.Port) + "/" + c.Database.Dbname + "?sslmode=" + c.Database.SSLMode
}

// GetLLMModelName returns the model name from the LLM_MODEL environment variable.
// Example: "openai/gpt-4o", "anthropic/claude-3-5-sonnet"
func (c *LLM) GetLLMModelName() string {
	return os.Getenv("LLM_MODEL")
}

// GetEmbedModelName returns the model name from the EMBED_MODEL environment variable.
// Example: "nvidia/llama-nemotron-embed-vl-1b-v2:free"
func (c *LLM) GetEmbedModelName() string {
	return os.Getenv("EMBED_MODEL")
}

// GetProviderName returns the provider name from the LLM_PROVIDER environment variable.
// Supported values: "openai", "anthropic"
func (c *LLM) GetProviderName() ModelProvider {
	return ModelProvider(os.Getenv("LLM_PROVIDER"))
}

// sharedHTTPClient is a process-wide HTTP client with a tuned connection pool.
// All non-Bedrock LLM SDKs reuse it so TLS handshakes and TCP connections are
// amortized across requests, reducing time-to-first-token.
var SharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	},
}
