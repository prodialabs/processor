def embed(conditioner, prompt, negative_prompt):
    prompt_embeds, pooled_prompt_embeds = conditioner(prompt)
    negative_prompt_embeds, negative_pooled_prompt_embeds = conditioner(negative_prompt)
    [prompt_embeds, negative_prompt_embeds] = conditioner.pad_conditioning_tensors_to_same_length([
        prompt_embeds,
        negative_prompt_embeds,
    ])

    return {
            'prompt_embeds': prompt_embeds,
            'pooled_prompt_embeds': pooled_prompt_embeds,
            'negative_prompt_embeds': negative_prompt_embeds,
            'negative_pooled_prompt_embeds': negative_pooled_prompt_embeds,
    }
