def parse_lora_slug(slug):
    parts = slug.rsplit('@', 1)
    if len(parts) == 1:
        return (parts[0], 'main')
    return (parts[0], parts[1])
