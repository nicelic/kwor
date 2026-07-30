# kwor

**An Advanced Web Panel • Built on SagerNet/Sing-Box and Mihomo**

Modified from [S-UI](https://github.com/alireza0/s-ui).

[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)

> **Disclaimer:** This project is only for personal learning and communication, please do not use it for illegal purposes, please do not use it in a production environment.

## Quick Overview

| Features                                | Enable?            |
| --------------------------------------- | :----------------: |
| Multi-Protocol                          | :heavy_check_mark: |
| Multi-Language                          | :heavy_check_mark: |
| Multi-Client/Inbound                    | :heavy_check_mark: |
| Dual Core (sing-box + mihomo)           | :heavy_check_mark: |
| Advanced Traffic Routing Interface      | :heavy_check_mark: |
| Client & Traffic & System Status        | :heavy_check_mark: |
| Subscription Service (link/json + info) | :heavy_check_mark: |
| Dark/Light Theme                        | :heavy_check_mark: |
| API Interface                           | :heavy_check_mark: |

## Supported Platforms

| Platform | Architecture   | Status         |
| -------- | -------------- | -------------- |
| Linux    | amd64, arm64   | ✅ Supported    |
| Docker (Linux host) | amd64, arm64   | ✅ Supported    |

## Default Installation Information

- Default Install Directory: `/opt/kwor`
- Panel Port: 8888
- Panel Path: /app/
- Subscription Port: auto-selected on first initialization from `25000-65000` with local TCP/UDP availability checks
- Subscription Path: auto-generated random path on first initialization
- Admin Credentials: interactive setup is handled by `kwor start` on first install

## Install & Upgrade to Latest Version

### Linux

```sh
bash <(curl -Ls https://raw.githubusercontent.com/nicelic/kwor/main/install.sh)
```

The installer behavior is:

1. Detect a running `kwor` process and upgrade in its current directory when possible.
2. If no process is running, detect `kwor.service` and reuse the directory encoded in `ExecStart` or `WorkingDirectory`.
3. If neither exists, perform a fresh install into `/opt/kwor`.
4. Reuse the program's built-in `kwor stop` and `kwor start` flow for upgrades and first-run setup.
5. Runtime support files `install.sh` and `kwor.service` are stored under `<binary_dir>/Promanager_data/` (default fresh install: `/opt/kwor/Promanager_data/`), and legacy copies next to the binary are auto-migrated on upgrade.

### Install legacy Version

To install a specific version, add the version tag to the end of the command, e.g. `v1.5.7`:

```sh
VERSION=v1.5.7 && bash <(curl -Ls https://raw.githubusercontent.com/nicelic/kwor/$VERSION/install.sh) $VERSION
```

The installer also accepts a bare version such as `1.5.7` and normalizes it to `v1.5.7`.

## Manual installation (Linux)

1. Get the latest release for your architecture from GitHub:
   [https://github.com/nicelic/kwor/releases/latest](https://github.com/nicelic/kwor/releases/latest)
   (`kwor-linux-amd64.tar.gz` or `kwor-linux-arm64.tar.gz`)
2. Extract the archive:
   ```sh
   tar -zxvf kwor-linux-amd64.tar.gz
   ```
3. Rename the binary for manual management:
   ```sh
   mv kwor kwor_amd64
   ```
4. Copy the binary into place:
   ```sh
   mkdir -p /opt/kwor
   cp -f kwor_amd64 /opt/kwor/
   chmod +x /opt/kwor/kwor_amd64
   ```
5. Start it with the built-in first-run flow:
   ```sh
   /opt/kwor/kwor_amd64 start
   ```
6. Common manual management commands:
   ```sh
   /opt/kwor/kwor_amd64 uri
   /opt/kwor/kwor_amd64 stop
   /opt/kwor/kwor_amd64 uninstall
   ```

## Uninstall

```sh
sudo -i
/opt/kwor/kwor_amd64 uninstall
```

Uninstall stops only processes and services whose exact executable path or
ownership record proves that they belong to kwor. It force-removes recognized
kwor resources even when their contents changed, continues with independent
resources after an individual failure, and prints every remaining failure for
retry. It never recursively removes the binary parent directory (for example
`/opt/kwor`), original files recorded as pre-existing, or host resources
without kwor ownership evidence.

Installer support files after installation live under `<binary_dir>/Promanager_data/`. For a default fresh install:

- `/opt/kwor/Promanager_data/install.sh`
- `/opt/kwor/Promanager_data/kwor.service`

## Install using Docker

<details>
   <summary>Click for details</summary>

### Step 1: Install Docker

Install Docker Engine and the Docker Compose plugin from the official Docker
documentation for your Linux distribution before continuing.

### Step 2: Run kwor

Docker deployment is intended for Linux hosts. `network_mode: host` and the
panel's nftables-based features require a Linux Docker engine plus
`CAP_NET_ADMIN`. Docker Desktop on Windows/macOS can run the panel in limited
bridge-network mode, but it will not provide the same host-network /
nftables behavior.

> Docker Compose method

```sh
mkdir kwor && cd kwor
wget -q https://raw.githubusercontent.com/nicelic/kwor/main/docker-compose.yml
docker compose up -d
```

容器首次启动会自动完成非交互初始化：

- 默认用户名是 `admin`
- 如果未传 `KWOR_BOOTSTRAP_PASSWORD`，容器会生成一次性随机密码并打印到容器日志
- 可选环境变量：`KWOR_BOOTSTRAP_USERNAME`、`KWOR_BOOTSTRAP_PASSWORD`、`KWOR_BOOTSTRAP_PANEL_PORT`、`KWOR_BOOTSTRAP_PANEL_PATH`、`KWOR_BOOTSTRAP_SUB_PORT`、`KWOR_BOOTSTRAP_SUB_PATH`

升级时请不要在面板内直接点“安装”。Docker 正确升级方式是拉取新镜像后重建容器，例如：

```sh
docker compose pull
docker compose up -d
```

证书管理中的 `acme.sh` 在 Docker 镜像里已包含 `curl`/`wget` 以及 `standalone` 模式常用的监听工具；如果你使用 `standalone` / `alpn` 挑战，仍需确保宿主机对应的 `80` / `443` 端口没有被其他进程占用。

> Plain docker run (Linux host network is recommended; kwor manages its own ports / nftables)

```sh
mkdir -p kwor/Promanager_data && cd kwor
docker run -itd \
    --cap-add NET_ADMIN \
    --security-opt no-new-privileges:true \
    --network host \
    -v $PWD/Promanager_data:/app/Promanager_data \
    --name kwor --restart=unless-stopped \
    ghcr.io/nicelic/kwor:v1.6.1
```

### Uninstall Docker

Do not run `uninstall` inside the container to remove a Docker deployment.
The image does not mount the Docker socket and only prints these host-side
instructions. Back up `Promanager_data` first when needed.

> Docker Compose bind mount

```sh
cd /path/to/kwor
docker compose down --remove-orphans
rm -rf -- ./Promanager_data
```

> Plain `docker run` bind mount or named volume

```sh
container=kwor
mount_type="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/app/Promanager_data"}}{{.Type}}{{end}}{{end}}' "$container")"
mount_name="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/app/Promanager_data"}}{{.Name}}{{end}}{{end}}' "$container")"
mount_source="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/app/Promanager_data"}}{{.Source}}{{end}}{{end}}' "$container")"
docker rm -f "$container"
case "$mount_type" in
  volume) [ -n "$mount_name" ] && docker volume rm "$mount_name" ;;
  bind) case "$mount_source" in ''|/|/opt|/usr|/var|/home|/root) printf '%s\n' "refusing unsafe bind mount: $mount_source" >&2; exit 1 ;; esac; rm -rf -- "$mount_source" ;;
  *) printf '%s\n' "no /app/Promanager_data mount found" >&2; exit 1 ;;
esac
```

Both paths remove only the exact `Promanager_data` bind mount or named volume;
they do not remove the deployment directory's parent.

### Build your own image

```sh
git clone https://github.com/nicelic/kwor
cd kwor
docker build -t kwor .
```

Run a locally built image with the same Linux host-network / capability
requirements:

```sh
mkdir -p kwor/Promanager_data && cd kwor
docker run -itd \
    --cap-add NET_ADMIN \
    --security-opt no-new-privileges:true \
    --network host \
    -v $PWD/Promanager_data:/app/Promanager_data \
    --name kwor --restart=unless-stopped \
    kwor
```

</details>

## Manual build (contribution)

<details>
   <summary>Click for details</summary>

### Windows (local development)

```bat
build.bat
```

### Linux / macOS

```sh
./build.sh
```

### Build steps explained

The repository root keeps both manuals as tracked source files:

- `User Manual.md`
- `使用手册.md`

The frontend source lives in `temp_frontend/`. A full build:

1. Builds the frontend:
   ```sh
   cd temp_frontend
   npm install
   npm run build
   cd ..
   ```
2. Copies the compiled frontend into `web/html/` (embedded into the Go binary):
   ```sh
   rm -fr web/html/*
   cp -R temp_frontend/dist/* web/html/
   ```
3. Builds the backend (pure Go, no CGO required):
   ```sh
   CGO_ENABLED=0 go build -ldflags "-w -s" -o kwor main.go
   ```
4. Runs it:
   ```sh
   ./kwor
   ```

### Release publishing

To publish the current source as a GitHub release without manually comparing history, first run the local Windows release build, then use the repo helper with its output directory:

```sh
node scripts/release-publish.mjs --push --assets-dir <local-release-assets-dir>
```

What it does:

1. Syncs package, Docker Compose, and README version metadata from `config/version`
2. Refuses to tag if release-relevant source files still have uncommitted changes
3. Creates `v<config/version>` at the current `HEAD`
4. Pushes `HEAD` to `main` and pushes the tag
5. Creates an empty draft GitHub Release, uploads and verifies the six local build assets, then publishes it
6. Confirms that publishing the GitHub Release has started the Docker workflow, without waiting for Docker to finish

Publishing the GitHub Release is always completed before Docker begins: the `release.published` workflow trigger starts the image build only after all six assets have been uploaded and the Release is published. Stable releases publish `v<version>`, `<version>`, and `latest` at `ghcr.io/nicelic/kwor`; prerelease versions do not move `latest`. A manual Docker workflow dispatch only validates the build and does not push an image. The local machine does not need Docker or `gh`; GitHub Actions builds and pushes the image.

To check an already published Docker image without pushing source, tags, releases, or images, run this from the matching Git clone:

```sh
node scripts/release-publish.mjs --verify-docker
```

If you intentionally need to move an existing version tag to the current commit, use:

```sh
node scripts/release-publish.mjs --push --retag --assets-dir <local-release-assets-dir>
```

`--retag` is only valid before the GitHub Release exists. This matters because the pushed source, local release assets, Docker image, and Compose example must describe the same version. The local publishing step finishes after the six GitHub assets are verified and Docker workflow startup is confirmed; it intentionally does not wait for Docker or GHCR completion. If Docker publication later fails, rerun or repair the matching Docker workflow from GitHub Actions instead of recreating the published Release.

</details>

## Languages

- English
- Farsi
- Vietnamese
- Chinese (Simplified)
- Chinese (Traditional)
- Russian

## Environment Variables

<details>
  <summary>Click for details</summary>

| Variable        |                      Type                      | Default  |
| --------------- | :--------------------------------------------: | :------- |
| KWOR_LOG_LEVEL  | `"debug"` \| `"info"` \| `"warn"` \| `"error"` | `"info"` |
| KWOR_DEBUG      |                   `boolean`                    | `false`  |
| KWOR_DB_FOLDER  |                    `string`                    | -        |

</details>

## SSL Certificate

<details>
  <summary>Click for details</summary>

kwor includes a built-in certificate manager (ACME / self-signed / import). For manual issuance with Certbot:

```sh
snap install core; snap refresh core
snap install --classic certbot
ln -s /snap/bin/certbot /usr/bin/certbot

certbot certonly --standalone --register-unsafely-without-email --non-interactive --agree-tos -d <Your Domain Name>
```

</details>
