# Handoff: Standalone AMD Video-Synthesis Pipeline

## Scope

Build a standalone local text-to-video pipeline on **wimpy**, using only the
AMD Radeon AI PRO R9700 (32 GB VRAM). Do not use the NVIDIA card. Do not add
video-generation models to Llama Hugs. Llama Hugs remains the text/LLM router
on `wimpy.home.lan:8080`.

Recommended stack:

- ComfyUI
- ROCm/PyTorch container
- Official native Wan2.1 ComfyUI workflow
- Wan2.1 T2V 1.3B for the first working pipeline
- 480p output initially; expand after a successful smoke test

This document is a future-work handoff. No setup is performed by writing it.

## Target layout

```text
/home/rahlquist/comfyui-video/
├── ComfyUI/
├── models/
│   ├── diffusion_models/
│   ├── text_encoders/
│   ├── vae/
│   └── clip_vision/
├── workflows/
└── outputs/
```

Keep the video stack separate from:

- `/opt/llama-hugs`
- `/home/rahlquist/.cache/llama.cpp`
- Llama Hugs systemd and port `8080`

## Hardware facts already established

- Host: CachyOS Linux, x86_64
- AMD GPU: Radeon AI PRO R9700, `gfx1201`, 32 GB VRAM
- ROCm packages are present; `rocm-hip-sdk` was observed at 7.2.4
- `/dev/kfd` and `/dev/dri/renderD128` are present
- User `rahlquist` belongs to `video` and `render`
- Docker, `uv`, and `ffmpeg` are installed
- Free disk observed during planning: approximately 367 GB
- Host Python is 3.14; do not build the ComfyUI environment around host Python

The R9700 is supported by AMD's ROCm compatibility material, but AMD's
ComfyUI guide targets Ubuntu/Python 3.10–3.12. Use a container to avoid host
Python and dependency contamination.

## Authoritative references

- ComfyUI Wan2.1 guide:
  https://docs.comfy.org/tutorials/video/wan/wan-video
- Official Wan2.1 repository:
  https://github.com/Wan-Video/Wan2.1
- Official Wan2.1 model organization:
  https://huggingface.co/Wan-AI
- ComfyUI repository:
  https://github.com/Comfy-Org/ComfyUI
- AMD ComfyUI/Radeon guide:
  https://rocm.blogs.amd.com/artificial-intelligence/comfyui-radeon-9000/README.html
- AMD ROCm compatibility matrix:
  https://rocm.docs.amd.com/en/docs-7.0.0/compatibility/compatibility-matrix.html

The ComfyUI guide explicitly documents a native Wan2.1 T2V 1.3B workflow and
places the required files under `models/diffusion_models`,
`models/text_encoders`, and `models/vae`. It identifies 8 GB VRAM as the
minimum for Wan2.1 1.3B; the R9700 has ample VRAM for the initial workflow.

## Phase 0 — preflight, no installation yet

Run on wimpy:

```bash
uname -a
python3 --version
docker --version
ffmpeg -version | head -1
ls -l /dev/kfd /dev/dri/renderD128
id

df -h /home
```

Confirm the user is in `video` and `render`. Confirm at least 50 GB is
available; 100 GB or more is preferable because model variants, caches, and
outputs grow quickly.

Confirm the AMD device identity before choosing the container pin:

```bash
rocminfo 2>/dev/null | grep -E 'Name:|Marketing Name:' | head -20
```

If `rocminfo` is unavailable on the host, verify GPU visibility from inside a
ROCm test container before installing ComfyUI.

## Phase 1 — create the isolated workspace

```bash
mkdir -p /home/rahlquist/comfyui-video/{models/{diffusion_models,text_encoders,vae,clip_vision},workflows,outputs}
cd /home/rahlquist/comfyui-video
```

Do not put this under `/opt/llama-hugs` and do not reuse the GGUF cache.

## Phase 2 — select and validate a ROCm container

First check the image tag rather than blindly pulling it. The AMD guide used a
ROCm 7.1/Python 3.12/PyTorch 2.6 image in its example:

```bash
docker manifest inspect \
  rocm/pytorch:rocm7.1_ubuntu24.04_py3.12_pytorch_release_2.6.0 \
  >/dev/null
```

If that exact tag is unavailable, select a currently published ROCm PyTorch
image whose ROCm version is compatible with the host driver and whose Python
version is 3.10–3.12. Record the exact chosen tag in the deployment notes.
Do not guess a tag and proceed.

Run a GPU visibility test. Replace `IMAGE` with the validated image tag:

```bash
IMAGE='rocm/pytorch:rocm7.1_ubuntu24.04_py3.12_pytorch_release_2.6.0'

docker run --rm \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add video \
  --group-add render \
  --ipc=host \
  -e HIP_VISIBLE_DEVICES=GPU-61fe9ba05af1939a \
  -e ROCR_VISIBLE_DEVICES=GPU-61fe9ba05af1939a \
  "$IMAGE" \
  python3 -c 'import torch; print(torch.cuda.is_available()); print(torch.version.hip); print(torch.cuda.get_device_name(0) if torch.cuda.is_available() else "no GPU")'
```

Expected result: CUDA compatibility layer reports available, HIP version is
non-empty, and the visible device is the AMD R9700. If the UUID is not
recognized inside the container, inspect `rocminfo` in the container and use
the device index that maps to the R9700; do not silently use the Raphael iGPU.

## Phase 3 — install ComfyUI in the container

Clone ComfyUI into the persistent workspace. The container is disposable; the
repository and models persist on the host volume:

```bash
cd /home/rahlquist/comfyui-video

docker run --rm -it \
  --device=/dev/kfd \
  --device=/dev/dri \
  --group-add video \
  --group-add render \
  --ipc=host \
  --network=host \
  -e HIP_VISIBLE_DEVICES=GPU-61fe9ba05af1939a \
  -e ROCR_VISIBLE_DEVICES=GPU-61fe9ba05af1939a \
  -v /home/rahlquist/comfyui-video:/workspace \
  -w /workspace \
  "$IMAGE" bash
```

Inside the container:

```bash
git clone https://github.com/comfyanonymous/ComfyUI.git
cd ComfyUI
python3 -m pip install --upgrade pip
python3 -m pip install -r requirements.txt
python3 main.py --listen 0.0.0.0 --port 8188
```

For the first bring-up, run interactively so import errors are visible. Do
not make it a systemd service until the smoke test succeeds.

Open locally or through the existing network path:

```text
http://wimpy.home.lan:8188
```

Check the server from another shell:

```bash
curl -s http://127.0.0.1:8188/system_stats
```

## Phase 4 — acquire the Wan2.1 T2V 1.3B components

Use the official ComfyUI-repackaged files and preserve the exact filenames
expected by the workflow. The guide lists these logical components:

```text
models/diffusion_models/
  wan2.1_t2v_1.3B_fp16.safetensors

models/text_encoders/
  umt5_xxl_fp8_e4m3fn_scaled.safetensors

models/vae/
  wan_2.1_vae.safetensors
```

The current official guide places the downloads in the
`Comfy-Org/Wan_2.1_ComfyUI_repackaged` repository. Inspect that repository's
`split_files` tree for the current exact file URLs before downloading; do not
invent filenames or use a random community checkpoint.

Preferred acquisition path from inside the container, after confirming the
exact URLs:

```bash
python3 -m pip install -U huggingface_hub

# Example shape only; replace URLs with the exact files confirmed from the
# current official split_files tree.
hf download Comfy-Org/Wan_2.1_ComfyUI_repackaged \
  --include 'split_files/diffusion_models/wan2.1_t2v_1.3B_fp16.safetensors' \
  --local-dir /workspace/model-downloads
```

Then place the files in the target folders:

```bash
install -Dm644 /workspace/model-downloads/split_files/diffusion_models/wan2.1_t2v_1.3B_fp16.safetensors \
  /workspace/models/diffusion_models/wan2.1_t2v_1.3B_fp16.safetensors

install -Dm644 /workspace/model-downloads/split_files/text_encoders/umt5_xxl_fp8_e4m3fn_scaled.safetensors \
  /workspace/models/text_encoders/umt5_xxl_fp8_e4m3fn_scaled.safetensors

install -Dm644 /workspace/model-downloads/split_files/vae/wan_2.1_vae.safetensors \
  /workspace/models/vae/wan_2.1_vae.safetensors
```

If the official repository uses a different current path or filename, adapt the
commands to the verified tree and record the change. Check that ComfyUI sees
the files after refresh/restart.

The T2V 1.3B workflow does not require CLIP Vision. Keep `clip_vision/` ready
for a later image-to-video workflow, but do not download unnecessary assets in
the first pass.

## Phase 5 — load the official workflow

In the ComfyUI web interface:

1. Update ComfyUI to a current version.
2. Open the Template Library.
3. Search for **Wan 2.1 Text to Video**.
4. Load the native workflow.
5. Set the diffusion loader to `wan2.1_t2v_1.3B_fp16.safetensors`.
6. Set the text encoder to `umt5_xxl_fp8_e4m3fn_scaled.safetensors`.
7. Set the VAE to `wan_2.1_vae.safetensors`.
8. Set a short text prompt.
9. Start with 480p and a short frame count.
10. Queue the workflow with `Ctrl+Enter` or the Run button.

If the template is absent, the ComfyUI version is too old or the workflow
library has changed. Update first; do not substitute an arbitrary workflow
until the official template has been checked.

## Phase 6 — first smoke test

Start conservatively:

```text
Resolution: 480p
Frames:     41 or 49
Steps:      20–30
Batch:      1
Output:     MP4 or the workflow's default video format
```

Acceptance criteria:

- ComfyUI starts without import failures
- AMD R9700 is the device doing the work
- Workflow queues and completes
- A playable video file appears under `/home/rahlquist/comfyui-video/outputs/`
- No CPU-only fallback
- No HIP out-of-memory or VAE decode failure
- Llama Hugs on port 8080 remains healthy throughout

Verify output:

```bash
find /home/rahlquist/comfyui-video/outputs -type f -printf '%s %p\n' | sort -n
ffprobe -v error -show_streams -show_format \
  /home/rahlquist/comfyui-video/outputs/<actual-output-file>
```

Monitor the AMD device from a second shell using whichever ROCm monitoring
command is available on wimpy. Do not use `nvidia-smi`; the NVIDIA card is out
of scope for this pipeline.

## Phase 7 — persistence after the smoke test

Only after a successful interactive test, create a persistent launcher. Two
valid options:

### Option A: container wrapper

Create a wrapper that launches the validated image with the same device,
volume, environment, and port arguments. Keep the image tag pinned.

### Option B: systemd user service

Use a user service for `rahlquist`, not the Llama Hugs system service. The unit
must:

- run the validated ROCm container
- expose port 8188 only as intended
- mount `/home/rahlquist/comfyui-video`
- set the R9700 device environment pin
- restart on failure
- leave Llama Hugs on 8080 untouched

Do not install a system-wide service until the interactive container has been
proven stable.

## Explicit non-goals

Do not do these in the first implementation:

- Do not use the NVIDIA card
- Do not attempt mixed-vendor multi-GPU execution
- Do not install Wan2.2 14B first
- Do not start with 720p
- Do not register Wan or ComfyUI components in Llama Hugs
- Do not place safetensors in `/home/rahlquist/.cache/llama.cpp`
- Do not modify Llama Hugs configuration, systemd, or port 8080
- Do not assume `pipeline_tag: text-to-video` means a complete video generator

## Upgrade path after the first successful video

1. Pin the working ComfyUI commit and container image.
2. Save the working workflow JSON under `workflows/`.
3. Add a small wrapper for prompt/output parameters.
4. Increase frame count before increasing resolution.
5. Test 720p only after 480p is reliable.
6. Evaluate Wan2.2, LTX-Video, or HunyuanVideo separately; do not replace the
   known-good Wan2.1 baseline until a new workflow passes the same smoke test.
7. Add optional post-processing only after generation is stable.

## Known caveats

- AMD's published ComfyUI instructions target Ubuntu and Python 3.10–3.12;
  wimpy is CachyOS/Python 3.14, which is why the container is preferred.
- ROCm/PyTorch container compatibility must be checked against the host driver.
- Wan video generation is memory- and time-intensive even when it fits VRAM.
- First execution may be slower because kernels/cache artifacts are created.
- A ComfyUI custom node executes Python code; inspect any third-party node
  before installing it.
- Official ComfyUI documentation says the 1.3B model is the low-VRAM entry
  point and recommends 480p for stability.

## Completion report template

When this handoff is executed, record:

```text
ComfyUI commit:
ROCm container image:
Host ROCm version:
AMD device identity:
Workflow file:
Model component checksums:
Resolution / frames / steps:
Output file:
Output ffprobe result:
Generation duration:
GPU verification:
Llama Hugs health during test:
Persistent launcher:
```

The final report must include the actual output path and real command results;
do not declare success from a server-start message alone.
