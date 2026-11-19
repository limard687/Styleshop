package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func main() {
	if _, err := os.Stat(".env"); err == nil {
		fmt.Println(".env exists; kept unchanged")
		return
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	b, err := os.ReadFile(".env.example")
	if err != nil {
		panic(err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		panic(err)
	}
	content := strings.ReplaceAll(string(b), "replace-with-at-least-32-random-characters", hex.EncodeToString(secret))
	f, err := os.OpenFile(".env", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		os.Remove(".env")
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	fmt.Println("Created .env with a random session secret. Demo mode enabled.")
}
