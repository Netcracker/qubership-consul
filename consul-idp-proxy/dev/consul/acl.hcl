acl {
  enabled        = true
  default_policy = "deny"
  tokens {
    # Dev only: the management token is "root".
    initial_management = "root"
  }
}