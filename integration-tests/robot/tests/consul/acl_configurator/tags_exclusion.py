def get_excluded_tags(environ) -> list:
    if not environ.get('CONSUL_ACL_CONFIGURATOR_HOST'):
        return ['acl-configurator', 'kv-configurator']
    return []