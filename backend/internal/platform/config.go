package platform

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	Address, DatabaseURL, AppOrigin string
	SecureCookies                   bool
}

func LoadConfig() (Config, error) {
	c := Config{Address: os.Getenv("HTTP_ADDR"), DatabaseURL: os.Getenv("DATABASE_URL")}
	if c.DatabaseURL == "" {
		c.DatabaseURL = os.Getenv("TS_POSTGRES_URL")
	}
	if c.DatabaseURL == "" {
		c.DatabaseURL = os.Getenv("TS_DATABASE_URL")
	}
	// Vercel's Go preset supplies PORT; it takes precedence over local settings.
	if port := os.Getenv("PORT"); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return c, fmt.Errorf("PORT must be an integer between 1 and 65535")
		}
		c.Address = ":" + port
	}
	if c.Address == "" {
		c.Address = ":8080"
	}
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return c, fmt.Errorf("HTTP_ADDR must be host:port: %w", err)
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL or TS_POSTGRES_URL is required")
	}
	c.AppOrigin = os.Getenv("APP_ORIGIN")
	if c.AppOrigin == "" {
		if os.Getenv("VERCEL") != "" {
			return c, fmt.Errorf("APP_ORIGIN is required on Vercel")
		}
		c.AppOrigin = "http://localhost:4200"
	}
	origin, err := url.Parse(c.AppOrigin)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return c, fmt.Errorf("APP_ORIGIN must be an HTTP(S) origin without a path")
	}
	c.SecureCookies = true
	if value := os.Getenv("SESSION_COOKIE_SECURE"); value != "" {
		c.SecureCookies, err = strconv.ParseBool(value)
		if err != nil {
			return c, fmt.Errorf("SESSION_COOKIE_SECURE must be true or false")
		}
	}
	local := origin.Hostname() == "localhost" || origin.Hostname() == "127.0.0.1" || origin.Hostname() == "::1"
	if !local && (origin.Scheme != "https" || !c.SecureCookies) {
		return c, fmt.Errorf("non-local APP_ORIGIN requires HTTPS and secure cookies")
	}
	return c, nil
}
