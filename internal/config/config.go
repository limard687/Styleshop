package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Config struct {
	Host, Port, DBPath, UploadDir, SessionSecret, DemoPassword string
	CookieSecure, SeedDemo                                     bool
}

func Load() (Config, error) {
	if f, err := os.Open(".env"); err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				return Config{}, fmt.Errorf("invalid .env line")
			}
			k = strings.TrimSpace(k)
			if _, exists := os.LookupEnv(k); !exists {
				os.Setenv(k, strings.Trim(strings.TrimSpace(v), "\"'"))
			}
		}
		if err := s.Err(); err != nil {
			return Config{}, err
		}
	} else if !os.IsNotExist(err) {
		return Config{}, err
	}
	get := func(k, fallback string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return fallback
	}
	c := Config{Host: get("HOST", "127.0.0.1"), Port: get("PORT", "8082"), DBPath: get("DB_PATH", "./data/styleshop.db"), UploadDir: get("UPLOAD_DIR", "./uploads"), SessionSecret: os.Getenv("SESSION_SECRET"), DemoPassword: os.Getenv("DEMO_PASSWORD")}
	var err error
	c.CookieSecure, err = strconv.ParseBool(get("COOKIE_SECURE", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid COOKIE_SECURE")
	}
	c.SeedDemo, err = strconv.ParseBool(get("SEED_DEMO", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid SEED_DEMO")
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("invalid PORT")
	}
	if len(c.SessionSecret) < 32 || strings.HasPrefix(c.SessionSecret, "replace-") {
		return c, fmt.Errorf("set SESSION_SECRET to at least 32 random characters")
	}
	if c.SeedDemo && (utf8.RuneCountInString(c.DemoPassword) < 8 || len(c.DemoPassword) > 72) {
		return c, fmt.Errorf("DEMO_PASSWORD must have at least 8 characters and fit the password length limit")
	}
	return c, nil
}
