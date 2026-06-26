import os

DEFAULT_SECRETS_DIR = '/etc/secrets/consul-integration-tests-pod-secrets'


def _secrets_dir(environ) -> str:
    return environ.get('INTEGRATION_TESTS_SECRETS_DIR', DEFAULT_SECRETS_DIR)


def check_that_parameters_are_presented(environ, *variable_names) -> bool:
    for variable in variable_names:
        if not environ.get(variable):
            return False
    return True


def secret_is_present(environ, name) -> bool:
    path = os.path.join(_secrets_dir(environ), name)
    return os.path.isfile(path) and os.path.getsize(path) > 0


def get_excluded_tags(environ) -> list:
    excluded_tags = []
    if not check_that_parameters_are_presented(environ,
                                               'CONSUL_BACKUP_DAEMON_HOST',
                                               'CONSUL_BACKUP_DAEMON_PORT',
                                               'DATACENTER_NAME'):
        excluded_tags.append('backup')
    if not (secret_is_present(environ, 'CONSUL_BACKUP_DAEMON_USERNAME')
            and secret_is_present(environ, 'CONSUL_BACKUP_DAEMON_PASSWORD')):
        excluded_tags.append('unauthorized_access')
    if environ.get('S3_ENABLED') != 'true' or not (
            secret_is_present(environ, 'S3_KEY_ID')
            and secret_is_present(environ, 'S3_KEY_SECRET')):
        excluded_tags.append('s3_storage')
    return excluded_tags
