# Coffer

A CLI tool for managing application configuration with GCP Secret Manager integration.

Coffer lets you store configuration in version-controlled YAML files while keeping secrets secure in GCP Secret Manager. It supports environment-specific overlays, secret references, and injects resolved configuration as environment variables.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install sultano/tap/coffer
```

### Install Script

```bash
curl -sfL https://raw.githubusercontent.com/sultano/coffer/main/install.sh | sh
```

To install to a custom directory:

```bash
INSTALL_DIR=/usr/local/bin curl -sfL https://raw.githubusercontent.com/sultano/coffer/main/install.sh | sh
```

### Go Install

```bash
go install github.com/sultano/coffer@latest
```

### Manual Download

Download the latest archive for your platform from [GitHub Releases](https://github.com/sultano/coffer/releases), extract it, and place the `coffer` binary in your `PATH`.

## Quick Start

```bash
# Initialize a new project
coffer init --gcp-project my-gcp-project

# Add a secret reference to your config
echo 'database:
  host: localhost
  password: ${secret:db-password}' >> config/base.yaml

# Set the secret in GCP
coffer secret set db-password "supersecret"

# Run your app with config injected
coffer run -- node server.js
```

## Usage

Coffer resolves your config and secrets, then injects them as environment variables into a child process. By default, only values containing secret references are injected as env vars. Use `--all` to inject all config values. You can also write the full resolved config to a file with `--config-file`. Coffer forwards signals and propagates the exit code.

### Process wrapping

```bash
coffer run --env prod -- node server.js
```

### Docker entrypoint

Install coffer in your image and use it as the entrypoint prefix. Secrets are resolved at container startup, not build time. Use `--config-file` to write the full resolved config for your app to read, while secrets are injected as env vars:

```dockerfile
ENTRYPOINT ["coffer", "run", "--config-file", "/app/config.json", "--"]
CMD ["node", "server.js"]
```

Set the environment using `COFFER_ENV`, `defaults.env` in `.coffer.yaml`, or override the entrypoint to pass `--env`:

```bash
# Dev — with local credentials mounted
docker run -e COFFER_ENV=dev \
  -v ~/.config/gcloud:/root/.config/gcloud \
  myapp

# Prod — with workload identity or service account key
docker run -e COFFER_ENV=prod myapp
```

### Generate config files

Write resolved config to a file for your app or tools like Docker Compose:

```bash
coffer resolve --env prod -f dotenv > .env
coffer resolve --env prod -f json > config.json
coffer resolve --env prod -f yaml > config.yaml
docker compose up
```

### Shell export

```bash
eval $(coffer resolve -f dotenv --env prod | sed 's/^/export /')
```

## Command Reference

| Command | Description |
|---------|-------------|
| `coffer init` | Initialize a new project |
| `coffer run -- <cmd>` | Run a command with secrets injected as env vars |
| `coffer resolve` | Output resolved config (JSON, YAML, or dotenv) |
| `coffer get <key>` | Get a single config value |
| `coffer check` | Validate all secrets exist in GCP |
| `coffer validate` | Validate config file syntax |
| `coffer info` | Show project configuration and status |
| `coffer secret list` | List secrets in GCP Secret Manager |
| `coffer secret get <name>` | Get a secret value |
| `coffer secret set <name>` | Create or update a secret |
| `coffer secret delete <name>` | Delete a secret |
| `coffer secret unused` | Find unreferenced secrets |
| `coffer secret import <file>` | Import secrets from a .env file |
| `coffer auth status` | Check GCP authentication status |
| `coffer auth login` | Authenticate to GCP |
| `coffer version` | Print version information |

### Global Flags

| Flag | Description |
|------|-------------|
| `-e, --env <name>` | Environment name (dev, staging, prod) |
| `-p, --path <dir>` | Path to project directory |
| `--dry-run` | Preview changes without applying |
| `--all` | Inject all config values as env vars (default: secrets only) |
| `--config-file <path>` | Write resolved config to a file (.json or .yaml) |
| `--no-color` | Disable colored output |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `COFFER_ENV` | Environment name (alternative to `--env` flag) |

## Configuration

### .coffer.yaml

```yaml
version: 1

config:
  path: ./config
  base: base.yaml

gcp:
  project: my-gcp-project
  secret_prefix: myapp-    # Optional: prefix for all secrets

environments:
  dev:
    gcp:
      project: my-gcp-project-dev
  prod:
    gcp:
      project: my-gcp-project-prod

env_mapping:               # Custom environment variable names
  database.host: DB_HOST
  database.password: DB_PASS

defaults:
  env: dev
```

### Config Overlay System

Configuration is merged in order (later files override earlier):

1. `base.yaml` - Base configuration
2. `{env}.yaml` - Environment-specific overrides (e.g., `dev.yaml`, `prod.yaml`)
3. `local.yaml` - Local development overrides (not committed)

### Secret References

Reference secrets in your config using `${secret:name}` syntax:

```yaml
database:
  password: ${secret:db-password}           # Simple reference
  api_key: ${secret:api-key@2}              # Specific version
  other: ${secret:projects/other/secrets/x} # Cross-project reference
```

### Secret Prefix

Use `secret_prefix` to namespace secrets when multiple services share a GCP project:

```yaml
gcp:
  project: shared-project
  secret_prefix: myservice-
```

With this config, `${secret:db-password}` fetches `myservice-db-password` from GCP.

### Environment Variables

Config keys are automatically converted to environment variables:

| Config Key | Environment Variable |
|------------|---------------------|
| `database.host` | `DATABASE_HOST` |
| `app.log_level` | `APP_LOG_LEVEL` |

Use `env_mapping` in `.coffer.yaml` to customize variable names.

## Commands

### coffer run

Run a command with configuration injected as environment variables. By default, only secret-containing values are injected:

```bash
coffer run -- npm start
coffer run --env prod -- ./deploy.sh
coffer run --all -- node server.js       # Inject all config values, not just secrets
coffer run --dry-run -- node server.js   # Preview env vars without running
```

Use `--config-file` to write the full resolved config to a file for your app to read:

```bash
coffer run --config-file config.json -- node server.js
coffer run --config-file config.yaml -- ./app
```

The file format is determined by the extension (`.json` or `.yaml`/`.yml`).

### coffer resolve

Output resolved configuration in different formats:

```bash
coffer resolve              # JSON (default)
coffer resolve -f yaml      # YAML
coffer resolve -f dotenv    # .env format
coffer resolve --env prod   # For a specific environment
```

### coffer get

Get a single configuration value:

```bash
coffer get database.host
coffer get app.log_level --env prod
```

### coffer check

Validate that all referenced secrets exist in GCP:

```bash
coffer check              # Check current/default environment
coffer check --env prod   # Check specific environment
coffer check --all        # Check all environments
```

### coffer validate

Validate configuration files for syntax errors:

```bash
coffer validate
```

Checks YAML syntax, secret reference format, and env_mapping keys.

### coffer secret

Manage secrets in GCP Secret Manager:

```bash
coffer secret list                        # List all secrets
coffer secret get db-password             # Get a secret value
coffer secret set db-password "value"     # Create/update a secret
coffer secret set api-key --from-file key.pem  # From file
coffer secret delete db-password          # Preview deletion
coffer secret delete db-password --yes    # Confirm deletion
coffer secret unused                      # Find unreferenced secrets
```

### coffer secret import

Import secrets from a .env file:

```bash
coffer secret import .env         # Preview what would be imported
coffer secret import .env --yes   # Import all secrets
```

Keys are converted: `DB_PASSWORD` becomes `db-password`.

### coffer auth

Manage GCP authentication:

```bash
coffer auth status   # Check authentication status
coffer auth login    # Authenticate to GCP (wraps gcloud)
```

## Authentication

Coffer uses Google Cloud Application Default Credentials (ADC):

```bash
# For local development
gcloud auth application-default login
# or
coffer auth login

# For CI/CD, use a service account
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

Required IAM roles:
- `roles/secretmanager.secretAccessor` - Read secrets
- `roles/secretmanager.admin` - Create/delete secrets (optional)

## CI/CD

```yaml
# GitHub Actions example
- name: Check secrets exist
  run: coffer check --env prod

- name: Deploy with secrets
  run: coffer run --env prod -- ./deploy.sh
```

## Development

```bash
git clone git@github.com:sultano/coffer.git
cd coffer
make setup    # Install git hooks
make build    # Build binary
make test     # Run tests
```

## License

MIT
