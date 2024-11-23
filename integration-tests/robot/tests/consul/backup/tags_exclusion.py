def check_that_parameters_are_presented(environ, *variable_names) -> bool:
    for variable in variable_names:
        if not environ.get(variable):
            return False
    return True


def get_excluded_tags(environ) -> list:
    excluded_tags = []
    if not check_that_parameters_are_presented(environ,
                                               'CONSUL_BACKUP_DAEMON_HOST',
                                               'CONSUL_BACKUP_DAEMON_PORT',
                                               'DATACENTER_NAME'):
        excluded_tags.append('backup')
    if not check_that_parameters_are_presented(environ,
                                               'CONSUL_BACKUP_DAEMON_USERNAME',
                                               'CONSUL_BACKUP_DAEMON_PASSWORD'):
        excluded_tags.append('unauthorized_access')
    if environ.get('S3_ENABLED') != 'true':
        excluded_tags.append('s3_storage')
    return excluded_tags