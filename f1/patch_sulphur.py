import json
p='/home/rahlquist/comfyui-video/workflows/sulphur_ltx23_t2v_distilled.json'
d=json.load(open(p))
for n in d['nodes']:
    t=n.get('type'); w=n.get('widgets_values')
    if t=='CheckpointLoaderSimple': n['widgets_values']=['sulphur_dev_fp8mixed.safetensors']
    elif t=='LTXAVTextEncoderLoader': n['widgets_values']=['gemma_3_12B_it_fp4_mixed.safetensors','sulphur_dev_fp8mixed.safetensors','default']
    elif t=='LTXVAudioVAELoader': n['widgets_values']=['LTX23_audio_vae_bf16.safetensors']
    elif t=='VAELoader': n['widgets_values']=['LTX23_video_vae_bf16.safetensors']
    elif t=='LoraLoaderModelOnly' and w and w[0]=='sulphur_final.safetensors': n['widgets_values']=['ltx-2.3-22b-distilled-lora-1.1_fro90_ceil72_condsafe.safetensors',0.7]
    elif t=='EmptyLTXVLatentVideo': n['widgets_values']=[768,512,17,1]
    elif t=='SaveVideo': n['widgets_values']=['video/Sulphur_2_t2v','auto','auto']
open(p,'w').write(json.dumps(d,ensure_ascii=False,indent=2))
print('patched')
