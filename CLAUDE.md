# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PicoClaw is an ultra-lightweight personal AI Assistant written in Go. It runs on minimal hardware (<10MB RAM) and supports multiple chat platforms (Telegram, Discord, QQ, DingTalk, LINE, WeCom). The agent can use various LLM providers and supports skills, memory, scheduled tasks, and multi-user workspaces.

## Common Commands

```bash
# Build
make build              # Build for current platform
make build-all         # Build for all platforms (Linux ARM64/x64, Darwin ARM64, Windows)
make run               # Build and run

# Development
make test              # Run all tests
make vet               # Run go vet
make fmt               # Format code
make check             # Run vet, fmt, and verify dependencies
make deps              # Download dependencies

# Run a single test
go test ./pkg/agent/... -run TestName

# Install
make install           # Install to ~/.local/bin
```

## Architecture

The codebase follows a modular package structure:

### Core Packages

- **pkg/agent**: Agent loop, context management, and registry. The agent core handles message processing and tool execution.
- **pkg/providers**: LLM provider implementations. Uses a factory pattern with model-centric configuration (`model_list`). Supports OpenAI-compatible protocol, Anthropic protocol, and custom providers (Claude CLI, Codex, GitHub Copilot, etc.)
- **pkg/channels**: Chat platform integrations (telegram, discord, qq, dingtalk, line, wecom, whatsapp, slack, feishu, maixcam). Each channel implements a common interface for receiving/sending messages.
- **pkg/tools**: Tools available to the agent (shell, filesystem, web search, cron, mailbox, spawn/subagent, memory search). Tools are registered in a registry and exposed to the LLM.
- **pkg/skills**: Skill management - allows extending agent capabilities through external packages from ClawHub.

### Supporting Packages

- **pkg/config**: Configuration loading (JSONC format), migration support
- **pkg/session**: Session management with persistent history
- **pkg/memory**: Long-term memory with Qdrant vector database
- **pkg/mailbox**: Cross-workspace communication for multi-user scenarios
- **pkg/family**: Family mode with per-user workspaces and safety filters
- **pkg/mcp**: Model Context Protocol integration for external tools
- **pkg/cron**: Scheduled tasks and reminders
- **pkg/heartbeat**: Periodic background tasks (reads HEARTBEAT.md)
- **pkg/bus**: Event bus for internal messaging
- **pkg/routing**: Message routing based on session keys and agent IDs
- **pkg/state**: Persistent state storage

### CLI Commands (cmd/picoclaw/)

- `agent`: Chat with the agent directly (one-shot or interactive)
- `gateway`: Start the gateway to receive messages from chat platforms
- `onboard`: Initialize configuration and workspace
- `cron`: Manage scheduled tasks
- `skills`: Manage skills (install, list, remove, search)
- `auth`: Manage authentication tokens
- `status`: Show PicoClaw status
- `migrate`: Migrate from OpenClaw to PicoClaw

## Key Concepts

### Workspace Structure

```
~/.picoclaw/
├── config.json         # Main configuration
├── workspace/          # Agent workspace
│   ├── sessions/      # Conversation history
│   ├── memory/        # Long-term memory (MEMORY.md)
│   ├── AGENTS.md     # Agent behavior instructions
│   ├── IDENTITY.md   # Agent identity
│   ├── SOUL.md       # Agent personality
│   ├── USER.md       # User preferences
│   ├── HEARTBEAT.md  # Periodic task prompts
│   └── cron/         # Scheduled jobs
├── skills/           # Installed skills
└── mailbox/          # Cross-workspace messages
```

### Provider Configuration

The new `model_list` configuration format uses `vendor/model` format (e.g., `anthropic/claude-sonnet-4-6`). This allows zero-code provider addition. The factory pattern in `pkg/providers/factory.go` routes to the correct provider based on the model prefix.

### Multi-User Isolation

Each user gets an isolated workspace with its own sessions, memory, and state. Messages are routed based on channel ID to the correct workspace context.

### Security

The agent runs sandboxed by default with `restrict_to_workspace: true`. File and command access is limited to the configured workspace directory.
