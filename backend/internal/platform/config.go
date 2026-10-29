package platform

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

type Config struct{ Address, DatabaseURL string }

func LoadConfig() (Config, error) {
	c := Config{Address: os.Getenv("HTTP_ADDR"), DatabaseURL: os.Getenv("DATABASE_URL")}
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
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}
