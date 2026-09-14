![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/rahlquist/llama-hugs/total)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/rahlquist/llama-hugs/go-ci.yml)
![GitHub Repo stars](https://img.shields.io/github/stars/rahlquist/llama-hugs)

# llama-hugs

Run multiple generative AI models on your machine and hot-swap between them on demand. llama-hugs works with any OpenAI and Anthropic API compatible server and is used by thousands of people to power their local AI workflows.

Built in Go for performance and simplicity, llama-hugs has zero dependencies and is incredibly easy to set up. Get started in minutes - just one binary and one configuration file.

## Features:

- ✅ Easy to deploy and configure: one binary, one configuration file. no external dependencies
- ✅ On-demand model switching for many local AI servers (llama.cpp + forks, vllm, stable-diffusion.cpp, audio.cpp, ComfyUI, etc.)
  - future proof, upgrade your inference servers at any time.
- ✅ OpenAI API supported endpoints:
  - `v1/completions`
  - `v1/chat/completions`
  - `v1/responses`
  - `v1/embeddings`
  - `v1/models` - list available models
  - `v1/audio/speech` ([#36](https://github.com/rahlquist/llama-hugs/issues/36))
  - `v1/audio/transcriptions` ([docs](https://github.com/rahlquist/llama-hugs/issues/41#issuecomment-2722637867))
  - `v1/audio/voices`
  - `v1/images/generations`
  - `v1/images/edits`
- ✅ Anthropic API supported endpoints:
  - `v1/messages`
  - `v1/messages/count_tokens`
- ✅ llama-server (llama.cpp) supported endpoints
  - `v1/rerank`, `v1/reranking`, `/rerank`
  - `/infill` - for code infilling
  - `/completion` - for completion endpoint
  - `/models` - list available models. same behavior as `v1/models`
  - `/props` - requires `?model={model_id}` query parameter to be provided. The autoload parameter is not supported and will be ignored.
- ✅ SDAPI via [stable-diffusion.cpp's server](https://github.com/leejet/stable-diffusion.cpp/tree/master/examples/server)
  - `/sdapi/v1/txt2img`
  - `/sdapi/v1/img2img`
  - `/sdapi/v1/loras` - requires `model` in request body to fetch the correct loras
- ✅ [audio.cpp](https://github.com/0xShug0/audio.cpp) supported [extra endpoints](https://github.com/0xShug0/audio.cpp/blob/main/app/server/README.md#post-v1tasksrun)
  - `/audioapi/v1/tasks/run`
- ✅ `/comfyui/` - ComfyUI custom endpoint ([#1001](https://github.com/rahlquist/llama-hugs/issues/1001)) for more reliable swapping
- ✅ llama-hugs API
  - `/ui` - web UI
  - `/upstream/:model_id` - direct access to upstream server ([demo](https://github.com/rahlquist/llama-hugs/pull/31))
  - `/running` - list currently running models ([#61](https://github.com/rahlquist/llama-hugs/issues/61))
  - `POST /api/models/unload` - manually unload all running models ([#58](https://github.com/rahlquist/llama-hugs/issues/58))
  - `POST /api/models/unload/:model_id` - unload a specific model
  - `GET /api/profiles` - list configured profiles and the active selection
  - `PUT /api/profiles/active` - activate a profile or select none
  - `/logs` - remote log monitoring
    - `GET /logs` returns buffered plain text logs.
      - If `Accept: text/html` is sent, `/logs` redirects to `/ui/`.
    - `GET /logs/stream` keeps the connection open for live log streaming.
      - Stream endpoints send buffered history first by default; add `?no-history` to stream only new lines.
    - `GET /logs/stream/proxy` streams proxy logs only.
    - `GET /logs/stream/upstream` streams upstream process logs only.
    - `GET /logs/stream/{model_id}` streams logs for one model (including IDs with slashes, like `author/model`).
  - `/health` - just returns "OK"
  - `/metrics` - system and GPU metrics for prometheus
- ✅ API Key support - define keys to restrict access to API endpoints
- ✅ Customization
  - Switch model ID routing at runtime with profiles
  - Run concurrent models with a custom DSL swap matrix ([#643](https://github.com/rahlquist/llama-hugs/issues/643))
  - Automatic unloading of models after timeout by setting a `ttl`
  - Docker and Podman support using `cmd` and `cmdStop` together
  - Preload models on startup with `hooks` ([#235](https://github.com/rahlquist/llama-hugs/pull/235))
  - Apply filters to requests to control inference with `stripParams`, `setParams` and `setParamsByID`

### Web UI

llama-hugs includes a real time web interface with a playground for testing out all sorts of local models:

<img width="1094" height="667" alt="image" src="https://github.com/user-attachments/assets/a79b3cea-5ee1-45f1-8db9-5f5331690e64" />

View detailed token metrics:

<img width="1090" height="672" alt="image" src="https://github.com/user-attachments/assets/145f4ece-af2f-4a45-a3c1-45ae5d3c7e7f" />

Inspect request and responses:

<img width="1078" height="668" alt="image" src="https://github.com/user-attachments/assets/947cda4f-9aa1-4fa5-a550-5c469968c1d9" />

Manually load and unload models:

<img width="1088" height="659" alt="image" src="https://github.com/user-attachments/assets/b6b850f3-c5b0-4c14-ba90-be2de25b51c7" />

Real time log streaming:

<img width="1087" height="668" alt="image" src="https://github.com/user-attachments/assets/9bb0c362-862c-4e68-820c-4c977fc9de4e" />

## Installation

llama-hugs can be installed in multiple ways

1. Docker
2. Homebrew (macOS and Linux)
3. MacPorts (macOS)
4. WinGet
5. From release binaries
6. From source

### Docker Install ([download images](https://github.com/rahlquist/llama-hugs/pkgs/container/llama-hugs))

Two types of container images are built nightly for llama-hugs:

1. A unified container with llama-server, ik-llama-server, stable-diffusion.cpp, whisper.cpp and llama-hugs built from source. This is only available for cuda and vulkan but has more capabilities. This one is recommended for use.
2. A legacy image that is based on llama.cpp's images and llama-hugs copied into the container. Use this one if you prefer to stay close to llama.cpp's container images.

#### Unified container (Recommended)

```shell
$ docker pull ghcr.io/rahlquist/llama-hugs:unified-cuda

# run with a custom configuration and models directory
$ docker run -it --rm --runtime nvidia -p 9292:8080 \
 -v /path/to/models:/models \
 -v /path/to/custom/config.yaml:/etc/llama-hugs/config/config.yaml \
 ghcr.io/rahlquist/llama-hugs:unified-cuda
```

#### Legacy container

```shell
$ docker pull ghcr.io/rahlquist/llama-hugs:cuda

# run with a custom configuration and models directory
$ docker run -it --rm --runtime nvidia -p 9292:8080 \
 -v /path/to/models:/models \
 -v /path/to/custom/config.yaml:/app/config.yaml \
 ghcr.io/rahlquist/llama-hugs:cuda
```

<details>
<summary>
more examples
</summary>

```shell
# pull latest images per platform
docker pull ghcr.io/rahlquist/llama-hugs:cpu
docker pull ghcr.io/rahlquist/llama-hugs:cuda
docker pull ghcr.io/rahlquist/llama-hugs:vulkan
docker pull ghcr.io/rahlquist/llama-hugs:intel
docker pull ghcr.io/rahlquist/llama-hugs:musa

# tagged llama-hugs, platform and llama-server version images
docker pull ghcr.io/rahlquist/llama-hugs:v166-cuda-b6795

# non-root cuda
docker pull ghcr.io/rahlquist/llama-hugs:cuda-non-root

```

</details>

### Homebrew Install (macOS/Linux)

```shell
brew tap rahlquist/llama-hugs
brew install llama-hugs
llama-hugs --config path/to/config.yaml --listen localhost:8080
```

### MacPorts (macOS)

> [!NOTE]
> Maintained by MacPorts community - [llama-hugs port](https://ports.macports.org/port/llama-hugs). It is not an official part of llama-hugs.

```shell
sudo port install llama-hugs
llama-hugs --config path/to/config.yaml --listen localhost:8080
```

### WinGet Install (Windows)

> [!NOTE]
> WinGet is maintained by community contributor [Dvd-Znf](https://github.com/Dvd-Znf) ([#327](https://github.com/rahlquist/llama-hugs/issues/327)). It is not an official part of llama-hugs.

```shell
# install
C:\> winget install llama-hugs

# upgrade
C:\> winget upgrade llama-hugs
```

### Pre-built Binaries

Binaries are available on the [release](https://github.com/rahlquist/llama-hugs/releases) page for Linux, Mac, Windows and FreeBSD.

### Building from source

1. Building requires Go and Node.js (for UI).
1. `git clone https://github.com/rahlquist/llama-hugs.git`
1. `make clean all`
1. look in the `build/` subdirectory for the llama-hugs binary

## Configuration

```yaml
# minimum viable config.yaml

models:
  model1:
    cmd: llama-server --port ${PORT} --model /path/to/model.gguf
```

That's all you need to get started:

1. `models` - holds all model configurations
2. `model1` - the ID used in API calls
3. `cmd` - the command to run to start the server.
4. `${PORT}` - an automatically assigned port number

Almost all configuration settings are optional and can be added one step at a time:

- Advanced features
  - `matrix` to run concurrent models with a custom swap logic DSL
  - `hooks` to run things on startup
  - `macros` reusable snippets
- Model customization
  - `ttl` to automatically unload models
  - `unloadTimeout` to tune graceful unloads (manual, API and `ttl` expiry)
  - `aliases` to use familiar model names (e.g., "gpt-4o-mini")
  - `env` to pass custom environment variables to inference servers
  - `cmdStop` gracefully stop Docker/Podman containers
  - `useModelName` to override model names sent to upstream servers
  - `${PORT}` automatic port variables for dynamic port assignment
  - `filters` rewrite parts of requests before sending to the upstream server

See the [configuration documentation](docs/configuration.md) for all options.

## How does llama-hugs work?

When a request is made to an OpenAI compatible endpoint, llama-hugs will extract the `model` value and load the appropriate server configuration to serve it. If the wrong upstream server is running, it will be replaced with the correct one. This is where the "swap" part comes in. The upstream server is automatically swapped to handle the request correctly.

In the most basic configuration llama-hugs handles one model at a time. For more advanced use cases, using a `matrix` allows multiple models to be loaded at the same time. You have complete control over how your system resources are used.

## Reverse Proxy Configuration (nginx)

If you deploy llama-hugs behind nginx, disable response buffering for streaming endpoints. By default, nginx buffers responses which breaks Server‑Sent Events (SSE) and streaming chat completion. ([#236](https://github.com/rahlquist/llama-hugs/issues/236))

Recommended nginx configuration snippets:

```nginx
# SSE for UI events/logs
location /api/events {
    proxy_pass http://your-llama-hugs-backend;
    proxy_buffering off;
    proxy_cache off;
}

# Streaming chat completions (stream=true)
location /v1/chat/completions {
    proxy_pass http://your-llama-hugs-backend;
    proxy_buffering off;
    proxy_cache off;
}
```

As a safeguard, llama-hugs also sets `X-Accel-Buffering: no` on SSE responses. However, explicitly disabling `proxy_buffering` at your reverse proxy is still recommended for reliable streaming behavior.

## Monitoring Logs on the CLI

```sh
# sends up to the last 10KB of logs
$ curl http://host/logs

# streams combined logs
curl -Ns http://host/logs/stream

# stream llama-hugs's proxy status logs
curl -Ns http://host/logs/stream/proxy

# stream logs from upstream processes that llama-hugs loads
curl -Ns http://host/logs/stream/upstream

# stream logs only from a specific model
curl -Ns http://host/logs/stream/{model_id}

# stream and filter logs with linux pipes
curl -Ns http://host/logs/stream | grep 'eval time'

# appending ?no-history will disable sending buffered history first
curl -Ns 'http://host/logs/stream?no-history'
```

## Do I need to use llama.cpp's server (llama-server)?

Any OpenAI compatible server would work. llama-hugs was originally designed for llama-server and it is the best supported.

For Python based inference servers like vllm or tabbyAPI it is recommended to run them via podman or docker. This provides clean environment isolation as well as responding correctly to `SIGTERM` signals for proper shutdown.

## GitHub Actions

The repository previously contained these scheduled workflows:

- **Build Containers** (`.github/workflows/containers.yml`) — built and published CPU, ROCm, CUDA, CUDA 13, Intel, MUSA, and Vulkan container images.
- **Build Unified Docker Image** (`.github/workflows/unified-docker.yml`) — built and optionally published unified CUDA and Vulkan images.
- **Close inactive issues** (`.github/workflows/closeinactive.yml`) — marked and closed inactive issues via `actions/stale`.

These repository-authored workflows were removed on 2026-08-30. The GitHub-managed Dependency Graph entry was left unchanged.
