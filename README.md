# kwor

**An Advanced Web Panel • Built on SagerNet/Sing-Box and Mihomo**

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
| Docker   | amd64, arm64   | ✅ Supported    |

## Default Installation Information

- Default Install Directory: `/opt/kwor`
- Panel Port: 8888
- Panel Path: /app/
- Subscription Port: 22780
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
3. Copy the binary into place:
   ```sh
   mkdir -p /opt/kwor
   cp -f kwor/kwor /opt/kwor/
   chmod +x /opt/kwor/kwor
   ```
4. Start it with the built-in first-run flow:
   ```sh
   /opt/kwor/kwor start
   ```
5. For later inspection:
   ```sh
   /opt/kwor/kwor uri
   /opt/kwor/kwor admin -show
   ```

## Uninstall

```sh
sudo -i
/opt/kwor/kwor uninstall
```

## Install using Docker

<details>
   <summary>Click for details</summary>

### Step 1: Install Docker

```sh
curl -fsSL https://get.docker.com | sh
```

### Step 2: Run kwor

> Docker Compose method

```sh
mkdir kwor && cd kwor
wget -q https://raw.githubusercontent.com/nicelic/kwor/main/docker-compose.yml
docker compose up -d
```

> Plain docker run (host network is recommended; kwor manages its own ports / nftables)

```sh
mkdir -p kwor/Promanager_data && cd kwor
docker run -itd \
    --network host \
    -v $PWD/Promanager_data:/app/Promanager_data \
    --name kwor --restart=unless-stopped \
    ghcr.io/nicelic/kwor:latest
```

### Build your own image

```sh
git clone https://github.com/nicelic/kwor
cd kwor
docker build -t kwor .
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
