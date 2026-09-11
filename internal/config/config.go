package config

import (
	"net"
	"net/url"
	"os"
)

type Config struct {
	HTTPAddr string

	PublicBaseURL string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
}

func Load() Config {
	return Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:18080"),
		DBHost:        getenv("DB_HOST", "localhost"),
		DBPort:        getenv("DB_PORT", "5432"),
		DBName:        getenv("DB_NAME", "cekdu_link"),
		DBUser:        getenv("DB_USER", "cekdu"),
		DBPassword:    getenv("DB_PASSWORD", "cekdu_local_dev"),
	}
}

// DatabaseURL builds a postgres DSN with all components properly escaped, so
// usernames, passwords, hostnames, and database names containing special
// characters (such as @ : / ? #) cannot corrupt the connection string.
func (c Config) DatabaseURL() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   net.JoinHostPort(c.DBHost, c.DBPort),
		Path:   "/" + c.DBName,
	}
	return u.String()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
