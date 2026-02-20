# AGENTS.md - Agent Coding Guidelines for PicoClaw

PicoClaw is a Go-based ultra-lightweight personal AI agent. This file provides guidelines for agents working on this codebase.

## Build, Lint, and Test Commands

### Build Commands
```bash
make build              # Build for current platform
make build-all          # Build for all platforms (linux, darwin, windows)
make install            # Install to ~/.local/bin
make clean              # Remove build artifacts
```

### Test Commands
```bash
make test               # Run all tests (go test ./...)
go test ./...          # Run all tests
go test -v ./...       # Run tests with verbose output
go test -v ./pkg/config -run TestName  # Run specific test
go test -v -count=1 ./...  # Run tests without cache
```

### Lint and Code Quality
```bash
make fmt               # Format code (go fmt ./...)
make vet               # Run go vet (go vet ./...)
make check             # Run deps, fmt, vet, test
golangci-lint run      # Run full linter (configured in .golangci.yaml)
```

### CI Pipeline
- PR checks: `go generate`, golangci-lint, fmt check, vet, test
- Main build: fmt check + build-all

## Code Style Guidelines

### General Principles
- Write clean, readable Go code
- Use `goimports` for formatting (enabled in .golangci.yaml)
- Maximum line length: 120 characters
- Use meaningful variable and function names

### Imports
Group imports in this order (blank line between groups):
1. Standard library
2. External packages (from go.mod)
3. Internal packages (github.com/sipeed/picoclaw/...)

```go
import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/caarlos0/env/v11"
    "github.com/sipeed/picoclaw/pkg/config"
    "github.com/sipeed/picoclaw/pkg/utils"
)
```

### Naming Conventions
- **Variables/Functions**: camelCase (e.g., `userName`, `loadConfig`)
- **Constants**: PascalCase or camelCase with meaningful names (e.g., `MaxRetries`, `defaultTimeout`)
- **Types/Structs**: PascalCase (e.g., `Config`, `AgentModelConfig`)
- **Interfaces**: PascalCase with "er" suffix when appropriate (e.g., `Reader`, `Writer`)
- **Packages**: lowercase, short, descriptive (e.g., `pkg/config`, `pkg/utils`)
- **Private fields**: camelCase starting with lowercase (e.g., `mu sync.RWMutex`)

### Types and Declarations
- Use explicit types for clarity in public APIs
- Use pointer receivers for methods that modify state (`func (c *Config) Method()`)
- Use value receivers for read-only methods (`func (c Config) String()`)
- Group related const declarations together
- Use `var` for package-level variables, prefer `:=` for local variables

### Error Handling
- Always handle errors explicitly - do not ignore with `_`
- Use `fmt.Errorf` with `%w` for error wrapping to preserve context
- Return errors early rather than nesting
- Check errors immediately after function calls

```go
// Good
cfg, err := config.LoadConfig(path)
if err != nil {
    return fmt.Errorf("loading config: %w", err)
}

// Avoid
if cfg, err := config.LoadConfig(path); err != nil {
    return err
}
```

### Testing
- Test files: `*_test.go` in the same package
- Use `testing.T` for all tests
- Use `testify/require` for assertions (already in go.mod)
- Name test functions: `Test<Package>_<Method>_<Scenario>`

```go
func TestConfig_LoadConfig_FileNotFound(t *testing.T) {
    cfg, err := LoadConfig("/nonexistent/path.json")
    require.Error(t, err)
    require.Nil(t, cfg)
}
```

### Configuration
- Use struct tags for JSON/environment variables
- Use `github.com/caarlos0/env/v11` for env variable parsing
- Use `github.com/tidwall/jsonc` for JSONC (commented JSON) support
- Provide `DefaultConfig()` functions for sensible defaults

### Concurrency
- Use `sync.RWMutex` for read-heavy scenarios
- Use `sync.Once` for one-time initialization
- Use `atomic` for simple counters
- Always use `defer` to unlock mutexes

### File Organization
- `cmd/picoclaw/`: CLI entry points
- `pkg/<feature>/`: Reusable packages
- One feature per package (e.g., `pkg/config`, `pkg/skills`, `pkg/tools`)
- Keep related files together

### Platform-Specific Code
- Use build tags: `//go:build linux`, `// +build linux`
- Suffix files with `_linux.go`, `_windows.go`, `_other.go`
- Use `runtime.GOOS` for runtime checks when needed

### Logging
- Use the `pkg/logger` package for structured logging
- Log levels: DEBUG, INFO, WARN, ERROR, FATAL
- Include contextual information in log fields

### Performance Considerations
- Reuse buffers where appropriate
- Use `sync.Pool` for frequently allocated objects
- Profile before optimizing

## Common Patterns

### Configuration Loading
```go
func LoadConfig(path string) (*Config, error) {
    cfg := DefaultConfig()
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return cfg, nil  // Return defaults if file doesn't exist
        }
        return nil, err
    }
    data = jsonc.ToJSON(data)
    if err := json.Unmarshal(data, cfg); err != nil {
        return nil, err
    }
    if err := env.Parse(cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

### Command-Line Interface
- Use simple switch statement in main for subcommands
- Group related commands in separate files under `cmd/picoclaw/`
- Provide help text for all commands

## Additional Resources
- Go standard: https://go.dev/
- Effective Go: https://go.dev/doc/effective_go
- Go Code Review Comments: https://github.com/golang/go/wiki/CodeReviewComments
