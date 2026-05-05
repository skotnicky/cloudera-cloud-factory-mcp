# Cloudera Cloud Factory MCP Server

A Model Context Protocol (MCP) server for managing Cloudera Cloud Factory resources from MCP-capable clients such as Claude Desktop and Cursor.

It supports project lifecycle operations, Kubernetes resources, application catalogs and installs, standalone VMs, cloud credentials, identity administration, backups, autoscaling, and related platform workflows.

[![CI](https://github.com/skotnicky/cloudera-cloud-factory-mcp/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/skotnicky/cloudera-cloud-factory-mcp/actions/workflows/ci.yml)
[![Release Workflow](https://github.com/skotnicky/cloudera-cloud-factory-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/skotnicky/cloudera-cloud-factory-mcp/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/skotnicky/cloudera-cloud-factory-mcp)](https://github.com/skotnicky/cloudera-cloud-factory-mcp/blob/main/go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/skotnicky/cloudera-cloud-factory-mcp)](https://goreportcard.com/report/github.com/skotnicky/cloudera-cloud-factory-mcp)

## Highlights

- Robot User authentication with scope-aware authorization
- Structured JSON responses for all tools
- Project and virtual cluster management
- Kubernetes deployment, patch, delete, and kubeconfig workflows
- Catalog, repository, and application lifecycle management
- Standalone VM, disk, IP, and profile management
- Cloud credential, image, flavor, and server management
- Identity, access profile, alerting, backup, autoscaling, and policy tooling

## Installation

### Option 1: Download pre-built binaries

Download the latest release from the [releases page](https://github.com/skotnicky/cloudera-cloud-factory-mcp/releases).

The `latest` release is published automatically from the `main` branch and replaced on each new push, so only the newest packaged build is kept.

#### Linux (x86_64)
```bash
curl -L https://github.com/skotnicky/cloudera-cloud-factory-mcp/releases/latest/download/cloudera-cloud-factory-mcp_Linux_x86_64.tar.gz | tar xz
sudo mv cloudera-cloud-factory-mcp /usr/local/bin/
```

#### macOS (Intel)
```bash
curl -L https://github.com/skotnicky/cloudera-cloud-factory-mcp/releases/latest/download/cloudera-cloud-factory-mcp_Darwin_x86_64.tar.gz | tar xz
sudo mv cloudera-cloud-factory-mcp /usr/local/bin/
```

#### macOS (Apple Silicon)
```bash
curl -L https://github.com/skotnicky/cloudera-cloud-factory-mcp/releases/latest/download/cloudera-cloud-factory-mcp_Darwin_arm64.tar.gz | tar xz
sudo mv cloudera-cloud-factory-mcp /usr/local/bin/
```

#### Windows (PowerShell)
```powershell
Invoke-WebRequest -Uri "https://github.com/skotnicky/cloudera-cloud-factory-mcp/releases/latest/download/cloudera-cloud-factory-mcp_Windows_x86_64.zip" -OutFile "cloudera-cloud-factory-mcp.zip"
Expand-Archive -Path "cloudera-cloud-factory-mcp.zip" -DestinationPath .
# Move cloudera-cloud-factory-mcp.exe to a directory on your PATH
```

### Option 2: Build from source

#### Prerequisites

- Go 1.25 or later
- Cloudera Cloud Factory Robot User credentials

```bash
git clone https://github.com/skotnicky/cloudera-cloud-factory-mcp
cd cloudera-cloud-factory-mcp
go build -o cloudera-cloud-factory-mcp .
```

### Option 3: Install with Go

```bash
go install github.com/skotnicky/cloudera-cloud-factory-mcp@latest
```

## Configuration

The server authenticates with **Robot User** credentials only. Use the legacy `TAIKUN_*` environment variable names expected by the upstream Go client.

```bash
export TAIKUN_ACCESS_KEY="your-robot-user-access-key"
export TAIKUN_SECRET_KEY="your-robot-user-secret-key"
export TAIKUN_API_HOST="api-latest.osc1.sjc.cloudera.com"  # Optional
export TAIKUN_DOMAIN_NAME=""                               # Optional
```

### Important authentication notes

- Email/password authentication is no longer supported by this server.
- If `TAIKUN_AUTH_MODE` is set, it is ignored.
- If `TAIKUN_API_HOST` is not set, the server defaults to `api-latest.osc1.sjc.cloudera.com`.

### Using a `.env` file

The binary does not automatically load `.env` files on startup. You can either export the variables in your shell or use the included helper target:

```bash
cp .env.example .env
make run-env
```

Equivalent manual shell usage:

```bash
set -a
source .env
set +a
./cloudera-cloud-factory-mcp
```

### Optional MCP lock defaults

You can pre-lock the MCP server to specific organization/project scopes at startup:

```bash
export MCP_LOCK_ORGANIZATION_IDS="12,34"
export MCP_LOCK_PROJECT_IDS="1001,1002"
```

These defaults can also be set directly in your MCP client config (`mcp.json`) via `env` or startup `args`.

Lock precedence and runtime behavior:

- Startup-configured IDs (from `MCP_LOCK_ORGANIZATION_IDS`, `MCP_LOCK_PROJECT_IDS`, or `--mcp-lock-*` args) are treated as **hard limits**.
- Runtime `mcp-lock` can only narrow scope within those hard limits.
- If runtime `mcp-lock` includes IDs outside hard limits, the request is rejected.
- If no hard limits and no runtime lock are set, MCP is unrestricted.
- Projects created through `create-project` and `create-virtual-cluster` are auto-added to an allowlist and treated as additional allowed project IDs alongside hard-limit projects.
- Auto-added project IDs are persisted in a local state file (`/tmp/cloudera_cloud_factory_mcp_lock_state.json` by default, configurable with `MCP_LOCK_STATE_PATH`).

## Usage

### Start the server

```bash
./cloudera-cloud-factory-mcp
```

The server communicates over MCP stdio transport.

### Print version information

```bash
./cloudera-cloud-factory-mcp --version
```

### Logs

Runtime logs are written to:

```text
/tmp/cloudera_cloud_factory_mcp_server.log
```

## MCP client configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "cloudera-cloud-factory": {
      "command": "/path/to/cloudera-cloud-factory-mcp",
      "args": [
        "--mcp-lock-project-ids=1001,1002"
      ],
      "env": {
        "TAIKUN_ACCESS_KEY": "your-robot-user-access-key",
        "TAIKUN_SECRET_KEY": "your-robot-user-secret-key",
        "TAIKUN_API_HOST": "api-latest.osc1.sjc.cloudera.com",
        "MCP_LOCK_ORGANIZATION_IDS": "12,34"
      }
    }
  }
}
```

Any MCP client that can launch a stdio server can use the same binary and environment variables.

### Claude Code — suppressing permission prompts

By default, Claude Code asks for approval before running each MCP tool call. To allow tools from this server without prompting, add them to the `permissions.allow` list in `.claude/settings.local.json` at the root of your working directory.

```json
{
  "permissions": {
    "allow": [
      "mcp__cloudera-cloud-factory__.*"
    ]
  },
  "enabledMcpjsonServers": [
    "cloudera-cloud-factory"
  ]
}
```

The permission key format is `mcp__<server-name>__<tool-name>`. The value is matched as a regular expression, so `mcp__cloudera-cloud-factory__.*` allows all tools from this server at once.

## Tool coverage

The server currently exposes tooling across these areas:

- Project management: create, list, inspect, commit, wait, and delete projects
- Cluster provisioning: create a full Kubernetes cluster via `create-cluster` (project + nodes + commit + optional wait)
- Virtual clusters: create, list, and delete virtual clusters
- Kubernetes: deploy YAML, patch resources, list resources, create or fetch kubeconfigs
- Cluster nodes and servers: add servers to projects, list them, and remove them
- Standalone VMs: create, inspect, manage IPs, operate power state, manage disks, and retrieve console/RDP/password metadata
- Standalone VM profiles: create, update, lock, and manage profile security group rules
- Applications and catalogs: repositories, catalog apps, installs, sync, uninstall, and wait flows
- Images and flavors: list, inspect, and bind images or flavors to projects
- Cloud credentials: list plus create or update provider-specific credentials
- Identity and access: domains, organizations, users, identity groups, and access profiles
- Platform services: alerting, backups, monitoring, AI assistant, policy, and spot settings
- Autoscaling: enable, inspect, update, and disable autoscaling

## Behavior notes

- `robot-user-capabilities` shows the current Robot User identity, scopes, and tool access.
- Scope-aware tools fail fast with a structured JSON error when the Robot User lacks required access.
- All tool responses are JSON, not free-form text.
- After adding Kubernetes servers or making standalone VM changes, call `commit-project` to provision them.
- `create-cluster` is the recommended path for end-to-end cluster creation because it avoids the empty-project-only flow.
- `create-cluster` accepts explicit `kubernetesProfileId` / `alertingProfileId`; when omitted it auto-selects only when a single deterministic profile exists, otherwise it returns an explicit JSON error asking for profile IDs.
- `create-cluster` can auto-pick node flavors from the target cloud credential's available flavors (or accept explicit flavor overrides).
- Standalone VM workflows may require binding images and flavors to the project first.
- In general, user requests for a "VM" or "server" map to standalone VM workflows, while "node" usually means adding capacity to a Kubernetes cluster.

## Development

Common local commands:

```bash
go test -v ./...
go build -o cloudera-cloud-factory-mcp .
make test
make test-all
make lint
```

## Support

- Create an issue in this repository
- Check the [Cloudera Cloud Factory documentation](https://docs.taikun.cloud/)
- Review the [MCP specification](https://modelcontextprotocol.io/)
