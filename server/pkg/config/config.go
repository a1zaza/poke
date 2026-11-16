package config

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

const (
	argon2Memory      uint32 = 19 * 1024
	argon2Iterations  uint32 = 2
	argon2Parallelism uint8  = 1
	argon2SaltLength         = 16
	argon2KeyLength          = 32
)

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

type Config struct {
	Auth   AuthConfig   `json:"auth"`
	System SystemConfig `json:"system"`
}

type AuthConfig struct {
	Password string `json:"password"`
}

type SystemConfig struct {
	Hostname   string           `json:"hostname,omitempty"`
	Logo       string           `json:"logo,omitempty"`
	Registries []RegistryConfig `json:"registries,omitempty"`
}

type RegistryConfig struct {
	Domain   string `json:"domain"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type secretsConfig struct {
	Registries map[string]string `json:"registries,omitempty"`
}

var (
	configFile    string
	secretsFile   string
	configLoadErr error
	mu            sync.RWMutex
	instance      *Config
)

func init() {
	// Determine config path similar to JS logic
	configDir := "/poke"
	configFile = filepath.Join(configDir, "config.json")
	secretsFile = filepath.Join(configDir, "secrets.json")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Try to create /poke
		if err := os.MkdirAll(configDir, 0755); err != nil {
			// Fallback to local ./data
			cwd, _ := os.Getwd()
			configDir = filepath.Join(cwd, "data")
			os.MkdirAll(configDir, 0755)
			configFile = filepath.Join(configDir, "config.json")
			secretsFile = filepath.Join(configDir, "secrets.json")
		}
	}
}

// Directory returns the actual persistence directory, including the development fallback.
func Directory() string {
	mu.RLock()
	defer mu.RUnlock()
	return filepath.Dir(configFile)
}

// SaveLogo uses the same atomic persistence mechanism as configuration files.
func SaveLogo(data []byte) error {
	return atomicWriteFile(filepath.Join(Directory(), "logo.png"), data, 0600)
}

func ResetLogo() error {
	err := os.Remove(filepath.Join(Directory(), "logo.png"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func Initialize() error {
	mu.Lock()
	defer mu.Unlock()
	getConfigLocked()
	return configLoadErr
}

func GetConfig() *Config {
	mu.Lock()
	defer mu.Unlock()
	return cloneConfig(getConfigLocked())
}

func getConfigLocked() *Config {
	if instance != nil {
		return instance
	}

	if err := ensurePrivateFile(configFile); err != nil {
		return failConfigLoad(fmt.Errorf("secure config file: %w", err))
	}
	if err := ensurePrivateFile(secretsFile); err != nil {
		return failConfigLoad(fmt.Errorf("secure secrets file: %w", err))
	}

	data, err := os.ReadFile(configFile)
	if os.IsNotExist(err) {
		plainPw, err := generateRandomString(12)
		if err != nil {
			return failConfigLoad(fmt.Errorf("generate initial password: %w", err))
		}
		hashedPw, err := hashPassword(plainPw)
		if err != nil {
			return failConfigLoad(fmt.Errorf("hash initial password: %w", err))
		}
		instance = &Config{
			Auth:   AuthConfig{Password: hashedPw},
			System: SystemConfig{},
		}
		if err := saveConfig(instance); err != nil {
			return failConfigLoad(fmt.Errorf("save initial config: %w", err))
		}

		fmt.Println("----------------------------------------")
		fmt.Printf("Initial Admin Password: %s\n", plainPw)
		fmt.Println("----------------------------------------")
		return instance
	}
	if err != nil {
		return failConfigLoad(fmt.Errorf("read config: %w", err))
	}

	instance = &Config{}
	if err := json.Unmarshal(data, instance); err != nil {
		return failConfigLoad(fmt.Errorf("parse config: %w", err))
	}

	hadInlineSecrets := hasInlineSecrets(instance)
	secrets, secretsExist, err := loadSecrets()
	if err != nil {
		return failConfigLoad(fmt.Errorf("load secrets: %w", err))
	}
	applySecrets(instance, secrets)

	if hadInlineSecrets || !secretsExist {
		if err := saveConfig(instance); err != nil {
			return failConfigLoad(fmt.Errorf("migrate config secrets: %w", err))
		}
	}

	return instance
}

func failConfigLoad(err error) *Config {
	configLoadErr = err
	instance = &Config{}
	fmt.Printf("Configuration error: %v\n", err)
	return instance
}

func VerifyPassword(input string) bool {
	mu.Lock()
	defer mu.Unlock()

	cfg := getConfigLocked()
	if configLoadErr != nil {
		return false
	}

	valid, needsMigration, err := verifyPasswordHash(input, cfg.Auth.Password)
	if err != nil || !valid {
		return false
	}
	if !needsMigration {
		return true
	}

	upgradedHash, err := hashPassword(input)
	if err != nil {
		fmt.Printf("Password migration error: %v\n", err)
		return true
	}
	legacyHash := cfg.Auth.Password
	cfg.Auth.Password = upgradedHash
	if err := saveConfig(cfg); err != nil {
		cfg.Auth.Password = legacyHash
		fmt.Printf("Password migration error: %v\n", err)
	}
	return true
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func simpleHash(password, saltHex string) string {
	dk := pbkdf2.Key([]byte(password), []byte(saltHex), 1000, 64, sha512.New)
	return fmt.Sprintf("%s:%s", saltHex, hex.EncodeToString(dk))
}

func verifyPasswordHash(password, stored string) (valid bool, needsMigration bool, err error) {
	if strings.HasPrefix(stored, "$argon2id$") {
		params, salt, expectedHash, err := parseArgon2Hash(stored)
		if err != nil {
			return false, false, err
		}
		actualHash := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(expectedHash)))
		return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, false, nil
	}

	valid, err = verifyLegacyPBKDF2(password, stored)
	return valid, valid, err
}

func parseArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, fmt.Errorf("unsupported Argon2id version")
	}
	if parts[2] != fmt.Sprintf("v=%d", version) {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id version")
	}
	var params argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.iterations, &params.parallelism); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id parameters")
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", params.memory, params.iterations, params.parallelism) {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id parameters")
	}
	if params.memory < 8*1024 || params.memory > 256*1024 || params.iterations < 1 || params.iterations > 10 || params.parallelism < 1 || params.parallelism > 16 {
		return argon2Params{}, nil, nil, fmt.Errorf("unsafe Argon2id parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id salt")
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedHash) < 16 || len(expectedHash) > 64 {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id hash")
	}
	return params, salt, expectedHash, nil
}

func verifyLegacyPBKDF2(password, stored string) (bool, error) {
	salt, encodedHash, ok := strings.Cut(stored, ":")
	if !ok || len(salt) != 32 {
		return false, fmt.Errorf("invalid legacy password hash")
	}
	expectedHash, err := hex.DecodeString(encodedHash)
	if err != nil || len(expectedHash) != 64 {
		return false, fmt.Errorf("invalid legacy password hash")
	}
	actualHash := pbkdf2.Key([]byte(password), []byte(salt), 1000, 64, sha512.New)
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func saveConfig(cfg *Config) error {
	publicConfig := cloneConfig(cfg)
	for i := range publicConfig.System.Registries {
		publicConfig.System.Registries[i].Password = ""
	}

	configData, err := json.MarshalIndent(publicConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	secretsData, err := json.MarshalIndent(extractSecrets(cfg), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	if err := atomicWriteFile(secretsFile, secretsData, 0600); err != nil {
		return fmt.Errorf("write secrets: %w", err)
	}
	if err := atomicWriteFile(configFile, configData, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ensurePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm() != 0600 {
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
		fmt.Printf("Configuration security: changed %s permissions to 0600\n", path)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer func() {
		if retErr != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempName)
		}
	}()

	if err := tempFile.Chmod(mode); err != nil {
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}

	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirHandle.Close()
	return dirHandle.Sync()
}

func loadSecrets() (secretsConfig, bool, error) {
	data, err := os.ReadFile(secretsFile)
	if os.IsNotExist(err) {
		return secretsConfig{}, false, nil
	}
	if err != nil {
		return secretsConfig{}, false, err
	}

	var secrets secretsConfig
	if err := json.Unmarshal(data, &secrets); err != nil {
		return secretsConfig{}, false, err
	}
	return secrets, true, nil
}

func hasInlineSecrets(cfg *Config) bool {
	for _, registry := range cfg.System.Registries {
		if registry.Password != "" {
			return true
		}
	}
	return false
}

func applySecrets(cfg *Config, secrets secretsConfig) {
	for i := range cfg.System.Registries {
		if password, ok := secrets.Registries[cfg.System.Registries[i].Domain]; ok {
			cfg.System.Registries[i].Password = password
		}
	}
}

func extractSecrets(cfg *Config) secretsConfig {
	secrets := secretsConfig{
		Registries: make(map[string]string),
	}
	for _, registry := range cfg.System.Registries {
		if registry.Password != "" {
			secrets.Registries[registry.Domain] = registry.Password
		}
	}
	return secrets
}

func cloneConfig(cfg *Config) *Config {
	cloned := *cfg
	cloned.System.Registries = append([]RegistryConfig(nil), cfg.System.Registries...)
	return &cloned
}

func UpdateConfig(updates map[string]interface{}) error {
	mu.Lock()
	defer mu.Unlock()

	cfg := getConfigLocked()
	if configLoadErr != nil {
		return fmt.Errorf("configuration unavailable: %w", configLoadErr)
	}
	previous := cloneConfig(cfg)

	// Update hostname
	if hostname, ok := updates["hostname"].(string); ok {
		cfg.System.Hostname = hostname
	}

	// Update logo
	if logo, ok := updates["logo"].(string); ok {
		cfg.System.Logo = logo
	}

	// Update password
	if password, ok := updates["password"].(string); ok && password != "" {
		hashedPassword, err := hashPassword(password)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		cfg.Auth.Password = hashedPassword
	}

	// Update Registries
	if regsRaw, ok := updates["registries"].([]interface{}); ok {
		var newRegs []RegistryConfig

		// Map existing passwords
		existingRegPasswords := make(map[string]string)
		for _, r := range cfg.System.Registries {
			existingRegPasswords[r.Domain] = r.Password
		}

		for _, r := range regsRaw {
			if rm, ok := r.(map[string]interface{}); ok {
				domain := fmt.Sprint(rm["domain"])
				password := ""
				if pw, ok := rm["password"].(string); ok && pw != "" {
					password = pw
				} else if len(existingRegPasswords[domain]) > 0 {
					// keep existing if not provided
					password = existingRegPasswords[domain]
				}

				reg := RegistryConfig{
					Domain:   domain,
					Username: fmt.Sprint(rm["username"]),
					Password: password,
				}
				newRegs = append(newRegs, reg)
			}
		}
		cfg.System.Registries = newRegs
	}

	if err := saveConfig(cfg); err != nil {
		instance = previous
		return err
	}
	return nil
}
