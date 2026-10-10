// Package config は利用者の設定（言語と LLM）を読み書きする。
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	Lang string `json:"lang"`
	// claude-code / codex / anthropic-api / openai-api / ollama。空なら未設定
	LLM string `json:"llm"`
}

// Path は設定ファイルの場所。Linux なら ~/.config/tour/config.json（XDG_CONFIG_HOME を尊重する）。
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tour", "config.json"), nil
}

// Load は設定を読む。まだなければ nil を返す。
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func Save(c *Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, append(b, '\n'), 0o644)
}
