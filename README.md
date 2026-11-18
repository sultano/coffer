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

# Set the secret in GCP (prompts without echoing the value)
coffer secret set db-password

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

Set the environment using `COFFER_ENV`, `defaults.env` in `.coffer.yaml`, or override the entrypoint to pass `--env`. In production the container gets Google credentials from the platform, such as the service account attached to a Cloud Run service or GKE Workload Identity:

```bash
docker run -e COFFER_ENV=prod myapp
```

For local development, don't mount your gcloud credentials into the container. Run coffer on the host and pass in only the variables the app needs, bypassing the image's coffer entrypoint:

```bash
coffer run --env dev -- docker run --rm -e DB_PASSWORD --entrypoint node myapp server.js
```

With Docker Compose, override the image's coffer entrypoint in the same way, and list the variables without values so they pass through from the environment coffer sets:

```yaml
services:
  app:
    image: myapp
    entrypoint: ["node", "server.js"]
    environment:
      - DB_PASSWORD
```

```bash
coffer run --env dev -- docker compose up
```

No Google credential enters the container this way, though the values are still visible to anyone who can run `docker inspect`.

### Generate config files

Prefer `coffer run`, which keeps secrets off disk. If a tool genuinely needs a file, `coffer resolve` can write one:

```bash
coffer resolve --env dev -f dotenv > .env
coffer resolve --env dev -f json > config.json
coffer resolve --env dev -f yaml > config.yaml
```

These files hold secrets in plaintext: keep them out of git (add them to `.gitignore`) and delete them when you are done. `coffer run --config-file` writes its file with mode 0600, but it also stays on disk after the command exits.

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
| `coffer auth status` | Check that Application Default Credentials are present |
| `coffer auth login` | Authenticate to GCP |
| `coffer version` | Print version information (also `coffer --version`) |

### Global Flags

| Flag | Description |
|------|-------------|
| `-e, --env <name>` | Environment name (dev, staging, prod) |
| `-p, --path <dir>` | Path to project directory |
| `--dry-run` | Preview changes without applying |
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

Every other `.yaml` file in the config directory is treated as an environment, alongside any declared under `environments:` in `.coffer.yaml`, so keep only config files there.

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
| `api.base-url` | `API_BASE_URL` |
| `3rd.party` | `_3RD_PARTY` |

Dots and dashes become underscores, a leading digit gets an underscore prefix, and any other character that is not valid in an env var name is dropped. Use `env_mapping` in `.coffer.yaml` to customize variable names.

Every resulting name, including `env_mapping` targets, must match `^[A-Za-z_][A-Za-z0-9_]*$`. Coffer stops with an error otherwise: a hyphenated mapping target or a key containing non-ASCII letters (such as `café.name`) is rejected rather than renamed, so give it an ASCII name in `env_mapping`.

If two config keys would produce the same variable name, coffer stops with an error naming both keys rather than picking one.

## Commands

### coffer run

Run a command with configuration injected as environment variables. By default, only secret-containing values are injected:

```bash
coffer run -- npm start
coffer run --env prod -- ./deploy.sh
coffer run --all -- node server.js       # Inject all config values, not just secrets
coffer run --dry-run -- node server.js   # Preview env vars (secrets masked) without running
```

Coffer's flags go before the command; everything from the command onwards is passed to it untouched, so `coffer run -- node app.js --env x` gives `--env x` to node. The `--` separator is optional but makes this explicit.

| Flag | Description |
|------|-------------|
| `--all` | Inject all config values as env vars (default: secrets only) |
| `-c, --config-file <path>` | Write the full resolved config to a file (`.json`, `.yaml` or `.yml`, mode 0600) |

Injected variables override any variable of the same name already set in the environment. Coffer forwards `SIGINT`, `SIGTERM`, `SIGHUP` and `SIGQUIT` to the command and exits with its exit code; if the command is killed by a signal, coffer exits with `128 + signal` (e.g. 143 for `SIGTERM`), as a shell would. Errors in coffer itself exit with 1.

Use `--config-file` to write the full resolved config to a file for your app to read:

```bash
coffer run --config-file config.json -- node server.js
coffer run --config-file config.yaml -- ./app
```

The file format is determined by the extension (`.json` or `.yaml`/`.yml`). The file is written with mode 0600 and is not deleted when the command exits.

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

Checks YAML syntax, secret reference format, env_mapping keys, and that no two config keys map to the same env var name.

### coffer secret

Manage secrets in GCP Secret Manager:

```bash
coffer secret list                        # List all secrets
coffer secret get db-password             # Get a secret value
coffer secret set db-password             # Create/update a secret (prompts, input hidden)
printf '%s' "$VALUE" | coffer secret set db-password   # From stdin
coffer secret set api-key --from-file key.pem  # From file
coffer secret delete db-password          # Preview deletion
coffer secret delete db-password --yes    # Confirm deletion
coffer secret unused                      # Find unreferenced secrets
```

A value can also be passed as a second argument (`coffer secret set db-password "value"`), but it is then recorded in your shell history and visible in process listings, so coffer prints a warning.

### coffer secret import

Import secrets from a .env file:

```bash
coffer secret import .env         # Preview what would be imported
coffer secret import .env --yes   # Import all secrets
```

Keys are converted: `DB_PASSWORD` becomes `db-password`. The preview masks every value. After a successful import, delete the source file: it still holds the secrets in plaintext.

### coffer auth

Manage GCP authentication:

```bash
coffer auth status   # Check that Application Default Credentials are present
coffer auth login    # Authenticate to GCP (wraps gcloud)
```

`auth status` does not test access to Secret Manager; run `coffer check --env <env>` for that.

## Authentication

Coffer uses Google Cloud Application Default Credentials (ADC):

```bash
# For local development
gcloud auth application-default login
# or
coffer auth login
```

In CI, avoid service account key files. See [CI/CD](#cicd) for keyless authentication from GitHub Actions.

Required IAM roles:
- `roles/secretmanager.secretAccessor` - Read secrets
- `roles/secretmanager.admin` - Create/delete secrets (optional)

## CI/CD

In GitHub Actions, use Workload Identity Federation through [`google-github-actions/auth`](https://github.com/google-github-actions/auth) instead of a service account key. The job needs `id-token: write`, and the action sets `GOOGLE_APPLICATION_CREDENTIALS`, which coffer picks up as ADC:

```yaml
permissions:
  contents: read
  id-token: write

steps:
  - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4.4.0

  - uses: google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093 # v3.0.0
    with:
      workload_identity_provider: projects/123456789/locations/global/workloadIdentityPools/github/providers/my-repo
      service_account: deployer@my-gcp-project-prod.iam.gserviceaccount.com

  - name: Check secrets exist
    run: coffer check --env prod

  - name: Deploy with secrets
    run: coffer run --env prod -- ./deploy.sh
```

Keep the checkout step before the auth step; otherwise later steps cannot authenticate.

## Security

Coffer keeps secrets out of your repository and in one place you control: access is granted per person or service account in IAM, revoking it takes effect everywhere at once, and Secret Manager can log every read.

It does not protect secrets from anything that runs as you:

- Malware, a compromised dependency or a coding agent with shell access can run coffer, or use your credentials directly, and fetch every secret you can.
- `coffer get`, `coffer resolve` and `coffer secret get` print plaintext, which can end up in terminal scrollback, CI logs and an AI agent's context.
- `coffer run` puts secrets in the child's environment, which other processes running as the same user can read and which every process the child starts inherits.

Recommendations:

- **Turn on audit logs.** Secret Manager Data Access audit logs are off by default, so reads are not recorded until you enable "Data Read" for the Secret Manager API: in the console under IAM & Admin > Audit Logs, with an `auditConfigs` entry in the project's IAM policy, or in Terraform with `google_project_iam_audit_config`. Cloud Logging bills for these logs.

  ```hcl
  resource "google_project_iam_audit_config" "secretmanager" {
    project = "my-gcp-project-prod"
    service = "secretmanager.googleapis.com"
    audit_log_config {
      log_type = "DATA_READ"
    }
  }
  ```

- **Use a separate GCP project per environment**, and don't grant `roles/secretmanager.secretAccessor` on production to laptops. An environment without its own `gcp.project` falls back to the top-level `gcp.project`, so a missing entry silently points at that project.
- **Narrow your local identity if you need to.** `gcloud auth application-default login --impersonate-service-account=SA_EMAIL` makes coffer act as that service account. The ADC file still contains your own user refresh token, so this limits what coffer uses, not what a process that reads the file can do.
- **Verify downloads.** Release archives carry GitHub build provenance attestations (releases built before they were added have none):

  ```bash
  gh attestation verify coffer_<version>_<os>_<arch>.tar.gz --repo sultano/coffer
  ```

  The install script only compares the archive with `checksums.txt` from the same release, and Homebrew does not check attestations. The Homebrew cask also removes the macOS quarantine flag, because the binaries are not notarised by Apple.

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
