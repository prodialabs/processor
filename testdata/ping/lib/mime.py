def mime_to_desired(mime):
    if mime == '*/*':
        return 'image/jpeg'

    return mime


def mime_to_format(mime):
    mimes = {
        'image/jpeg': 'JPEG',
        'image/webp': 'WEBP',
        'image/png': 'PNG',
    }

    return mimes.get(mime, 'PNG')
