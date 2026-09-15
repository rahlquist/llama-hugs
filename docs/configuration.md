# config.yaml

llama-hugs is designed to be very simple: one binary, one configuration file.

## minimal viable config

```yaml
models:
  model1:
    cmd: llama-server --port ${PORT} --model /path/to/model.gguf
```

This is enough to launch `llama-server` to serve `model1`. Of course, llama-hugs is about making it possible to serve many models:

```yaml
models:
  model1:
    cmd: llama-server --port ${PORT} -m /path/to/model.gguf
  model2:
    cmd: llama-server --port ${PORT} -m /path/to/another_model.gguf
  model3:
    cmd: llama-server --port ${PORT} -m /path/to/third_model.gguf
```

With this configuration models will be hot swapped and loaded on demand. The special `${PORT}` macro provides a unique port per model which is useful if you want to run multiple models at the same time with the `matrix` feature.

## Advanced control with `cmd`

llama-hugs is also about customizability. You can use any CLI flag available:

```yaml
models:
  model1:
    cmd: | # support for multi-line
      llama-server --PORT ${PORT} -m /path/to/model.gguf
      --ctx-size 8192
      --jinja
      --cache-type-k q8_0
      --cache-type-v q8_0
```

## Support for any OpenAI API compatible server

llama-hugs supports any OpenAI API compatible server. If you can run it on the CLI llama-hugs will be able to manage it. Even if it's run in Docker or Podman containers.

```yaml
models:
  "Q3-30B-CODER-VLLM":
    name: "Qwen3 30B Coder vllm AWQ (Q3-30B-CODER-VLLM)"
    # cmdStop provides a reliable way to stop containers
    cmdStop: docker stop vllm-coder
    cmd: |
      docker run --init --rm --name vllm-coder
        --runtime=nvidia --gpus '"device=2,3"'
        --shm-size=16g
        -v /mnt/nvme/vllm-cache:/root/.cache
        -v /mnt/ssd-extra/models:/models -p ${PORT}:8000
        vllm/vllm-openai:v0.10.0
        --model "/models/cpatonn/Qwen3-Coder-30B-A3B-Instruct-AWQ"
        --served-model-name "Q3-30B-CODER-VLLM"
        --enable-expert-parallel
        --swap-space 16
        --max-num-seqs 512
        --max-model-len 65536
        --max-seq-len-to-capture 65536
        --gpu-memory-utilization 0.9
        --tensor-parallel-size 2
        --trust-remote-code
``

## Docker/Podman runtime members

Any model whose `cmd` starts with a container runtime (`docker`, `podman`,
...) runs that container as the upstream. llama-hugs treats it as opaque —
it just waits for `:$PORT/health` to respond. This lets you swap between
inference *engines* (not just models) by pointing `cmd` at a different
image.

### Contract (verified on wimpy, 2026-09-15)

```yaml
models:
  "qwen3-8-27b-ktopt-cuda":
    ttl: 300
    env:
    - CUDA_VISIBLE_DEVICES=0
    capabilities:
      in: [text]
      out: [text]
      tools: true
    cmd: /usr/bin/docker run --rm --gpus all -v /home/rahlquist/kt-models:/models:ro -e PORT=${PORT} -e PROFILE=auto -e GPU_INDEX=0 -e CUDA_VISIBLE_DEVICES=0 -e ALIAS=qwen3-8-27b-ktopt-cuda -p ${PORT}:${PORT} ghcr.io/lrozewicz/kt-llama-cpp:cuda
```

**Rules:**

1. **`cmd` must be an absolute path.** Bare `docker` fails with
   `upstream command exited prematurely` — Go's `exec.Command` does not
   resolve via `$PATH` the way a shell would. Use `/usr/bin/docker` or
   `/usr/bin/podman`.

2. **`cmd` must be a single line.** A yaml block scalar (`cmd: |`) is
   preserved with literal `\n` characters, which the entrypoint receives
   as garbage. The vllm example above works because the block scalar's
   newlines collapse to spaces when parsed — but a `${PORT}`-dependent
   `-p` flag and env `-e` sequence is safer on one line.

3. **`${PORT}` is substituted everywhere** — in `-p ${PORT}:${PORT}`,
   env vars, flags. llama-hugs allocates a unique port per model.

4. **Do not declare `aliases:` equal to the model id.** `gen-config.py`
   auto-registers `model_id` sans the `hugs-` prefix as an alias. A
   duplicate causes `duplicate alias ... found` and the entry is refused.

5. **Mount models read-only** (`:ro`) when the entrypoint only reads them.
   The kt-llama-cpp entrypoint never writes to `/models`.

6. **Use `--rm`** so the container auto-removes on exit — no lingering
   containers after llama-hugs unloads the model via `ttl`.

7. **`cmdStop` is optional** — for named containers that need a graceful
   stop signal. With `--rm` and a `ttl`, llama-hugs sends SIGTERM to the
   process group; the container runtime handles the rest.

### When to use a docker member

- The model needs a forked inference server (e.g. KT-quantized GGUFs that
  only load in `lrozewicz/kt-llama.cpp`, not mainline `llama.cpp`).
- The model needs a different engine entirely (vllm, ComfyUI, audio.cpp).
- You want the entrypoint to self-manage VRAM profiling, KV cache
  compression, or speculative decoding drafter selection based on free
  GPU memory at launch time.

## Many more features..

llama-hugs supports many more features to customize how you want to manage your environment.

| Feature   | Description                                    |
| --------- | ---------------------------------------------- |
| `ttl`     | automatic unloading of models after a timeout  |
| `macros`  | reusable snippets to use in configurations     |
| `matrix`  | run multiple models at a time                  |
| `hooks`   | event driven functionality                     |
| `env`     | define environment variables per model         |
| `aliases` | serve a model with different names             |
| `filters` | modify requests before sending to the upstream |
| `profiles` | switch model ID replacements at runtime       |
| `...`     | And many more tweaks                           |

## Full Configuration Example

Check [config.example.yaml](https://github.com/mostlygeek/llama-hugs/blob/main/config.example.yaml) for the most up to date reference for all example configurations. It has grown quite complex but your favorite local LLM can help with a local configuration.