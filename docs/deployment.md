# CI/CD Deployment

This project deploys as three long-running services behind Docker Compose:

- `frontend`: Nginx serving the Vite build and proxying `/api` to the backend.
- `backend`: the Go API container.
- `postgres` and `redis`: stateful infrastructure with named Docker volumes.

## Production Server Setup

1. Install Docker and Docker Compose on the server.
2. Create a deployment directory, for example `/opt/lalalandshow`.
3. Copy `deploy/.env.prod.example` to `/opt/lalalandshow/.env`.
4. Fill every secret in `/opt/lalalandshow/.env`.

The deployment workflow copies these files to the server automatically:

- `docker-compose.prod.yml`
- `nginx.conf`
- `.env.prod.example`
- `remote-deploy.sh`
- `.release.env`

Do not store production secrets in GitHub source files. Keep them in the
server-side `.env` file and in GitHub repository secrets.

## GitHub Secrets

Configure these repository or environment secrets:

- `DEPLOY_HOST`: server hostname or IP.
- `DEPLOY_PORT`: SSH port, usually `22`.
- `DEPLOY_USER`: SSH user.
- `DEPLOY_SSH_KEY`: private key for the SSH user.
- `DEPLOY_PATH`: deployment directory, for example `/opt/lalalandshow`.

The workflow uses `GITHUB_TOKEN` to publish images to GHCR. If GHCR packages are
private, make sure the server can pull them. The simplest option is to run this
once on the server:

```bash
docker login ghcr.io
```

Use a GitHub PAT with package read access when prompted.

## Release Flow

Normal flow:

```text
push to main
  -> CI tests backend and builds frontend
  -> deployment workflow builds images
  -> images are pushed to GHCR
  -> server pulls the new image tag
  -> docker compose up -d
```

Manual deployment is available from the `Build and Deploy` workflow. Leave
`image_tag` empty to deploy the current commit SHA, or provide a tag for a
named release.

## Fast Local Deploy

For on-site debugging, you can build images on your computer and stream them to
the server without waiting for GitHub Actions:

```bash
DEPLOY_HOST=your-server \
DEPLOY_USER=root \
DEPLOY_PATH=/opt/lalalandshow \
deploy/local-deploy.sh
```

Optional environment variables:

- `DEPLOY_PORT`: SSH port, defaults to `22`.
- `IMAGE_TAG`: image tag, defaults to `local-<git-sha>`.
- `BACKEND_IMAGE`: local backend image name.
- `FRONTEND_IMAGE`: local frontend image name.
- `SSH_OPTS`: extra SSH options, for example `-i ~/.ssh/deploy_key`.

This path is intentionally for fast debugging. The GitHub Actions path should
remain the source of truth for production releases.

## Rollback

On the server, edit `.release.env` and set `IMAGE_TAG` to an older known-good
tag. Then run:

```bash
cd /opt/lalalandshow
./remote-deploy.sh
```

## Local Production Smoke

After GitHub publishes images, you can test production Compose locally:

```bash
cp deploy/.env.prod.example deploy/.env
# edit deploy/.env with local-safe secrets and real image names
cd deploy
docker compose --env-file .env -f docker-compose.prod.yml up -d
```

Open `http://localhost` and check `http://localhost/healthz`.
