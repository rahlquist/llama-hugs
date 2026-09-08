import json
out={
"44":{"class_type":"CheckpointLoaderSimple","inputs":{"ckpt_name":"sulphur_dev_fp8mixed.safetensors"}},
"5":{"class_type":"LTXAVTextEncoderLoader","inputs":{"text_encoder":"gemma_3_12B_it_fp4_mixed.safetensors","ckpt_name":"sulphur_dev_fp8mixed.safetensors","device":"cpu"}},
"30":{"class_type":"CLIPTextEncode","inputs":{"text":"a red fox walking through snow, cinematic, natural motion","clip":["5",0]}},
"41":{"class_type":"CLIPTextEncode","inputs":{"text":"blurry, static, distorted, low quality, text, watermark","clip":["5",0]}},
"31":{"class_type":"LTXVConditioning","inputs":{"positive":["30",0],"negative":["41",0],"frame_rate":8}},
"21":{"class_type":"EmptyLTXVLatentVideo","inputs":{"width":256,"height":256,"length":9,"batch_size":1}},
"47":{"class_type":"LTXVScheduler","inputs":{"steps":4,"max_shift":4.0,"base_shift":1.5,"stretch":True,"terminal":0.1,"latent":["21",0]}},
"17":{"class_type":"KSamplerSelect","inputs":{"sampler_name":"euler"}},
"42":{"class_type":"CFGGuider","inputs":{"model":["44",0],"positive":["31",0],"negative":["31",1],"cfg":1.0}},
"2":{"class_type":"RandomNoise","inputs":{"noise_seed":42}},
"36":{"class_type":"SamplerCustomAdvanced","inputs":{"noise":["2",0],"guider":["42",0],"sampler":["17",0],"sigmas":["47",0],"latent_image":["21",0]}},
"68":{"class_type":"VAEDecode","inputs":{"samples":["36",0],"vae":["44",2]}},
"38":{"class_type":"CreateVideo","inputs":{"images":["68",0],"fps":8}},
"45":{"class_type":"SaveVideo","inputs":{"video":["38",0],"filename_prefix":"video/Sulphur_diag","format":"auto","codec":"auto"}}
}
json.dump({'prompt':out,'client_id':'hermes-sulphur-diag'},open('/home/rahlquist/comfyui-video/sulphur_diag_payload.json','w'))
