# Terraform Provider for TP-Link Omada Controller

## Getting credentials

**Base URL**: the address you use to reach your Omada Controller's web UI, including
port, e.g. `https://192.168.1.1:8043` for a software controller, or your Omada Cloud-hosted
gateway/OC200's local address. This provider only talks to the controller's local API — it
does not use Omada Cloud.

**Username / password**: a local Omada Controller admin account (Settings → Controller →
Local Admin Account, or Settings → Admin Users on some versions). Any account with
administrator privileges on the target site works; a dedicated service account is
recommended over reusing a personal login. Environment variables `OMADA_USERNAME` and
`OMADA_PASSWORD` can be used instead of putting these in provider config.

**Open API client ID / secret (optional)**: create a Client-mode application under
Settings → Platform Integration → Open API. When `client_id` and `client_secret`
are configured, the provider authenticates public Open API operations with the
application's scoped access token and refreshes expiring tokens automatically.
Controller-internal web endpoints continue to use the classic username/password
session, including the internal endpoints whose paths begin with `/openapi`. If the
application credentials are omitted, public Open API operations also fall back to the
classic session. Use environment variables `OMADA_CLIENT_ID` and
`OMADA_CLIENT_SECRET` to keep the secret out of HCL.

**Site name or ID**: on the controller UI, the current site is shown in the top-left site
selector. Its name (e.g. `Default`) can be used directly in provider config as `site`; its ID
is visible in the browser URL after selecting the site (the segment following `/#/site/...`,
before the next `/`). Environment variable `OMADA_SITE` can be used instead.

**TLS**: if your controller uses a self-signed certificate, set `skip_tls_verify = true` (or
`OMADA_SKIP_TLS_VERIFY=true`).

## Provider configuration

```hcl
provider "omada" {
  base_url = "https://192.168.1.1:8043"
  username = "admin"
  password = "changeme"

  # Optional Open API application credentials.
  client_id     = "terraform"
  client_secret = "replace-with-the-open-api-app-secret"
  site     = "Default"
}
```

Full reference documentation is generated under [`docs/`](docs/), with runnable examples
in [`examples/`](examples/). `omada_vlan` manages the 802.1Q tag, gateway subnet and DHCP
scope. `omada_switch_port` manages an existing physical switch port, including its name,
applied profile, native network and link-speed settings. `omada_switch_port_profile`
manages the native and tagged VLAN membership of reusable switch port profiles.
`omada_ssid` and `omada_wlan_group` manage wireless networks, while
`omada_acl` manages gateway, switch and EAP access-control rules. Read-only
lookups and inventory data sources are available for sites, VLANs, switches,
port profiles, WLAN groups, SSIDs and ACLs.

## Installing

```hcl
terraform {
  required_providers {
    omada = {
      source  = "filipegalo/omada"
      version = "~> 0.3"
    }
  }
}
```

Both Terraform and OpenTofu resolve this address.

### Installing a local build

To run an unreleased build, install it into the implied local filesystem mirror:

```sh
make install       # this machine only
make install-all   # also cross-compiles linux_amd64, so a lock file can cover it
```

The version is derived from `git describe`, so a build between tags gets a distinct
prerelease version and never collides with a released one. Point the consuming config at
`local/filipegalo/omada` with that version to use it.

## Development

```sh
make test        # unit tests, no controller needed
make lint        # golangci-lint
make generate    # regenerates docs/ from the schema and examples/; CI fails if stale
make testacc     # acceptance tests against a REAL controller (creates and destroys objects)
```

Unit tests run against an in-process mock of the controller that enforces the same required
fields the real API does. Acceptance tests need `OMADA_URL`, `OMADA_USERNAME`,
`OMADA_PASSWORD` and `OMADA_SITE`, plus the per-suite variables documented at the top of each
`*_acc_test.go`.

## Releasing

Versions come from git tags, and [`CHANGELOG.md`](CHANGELOG.md) is maintained by hand.
Pushing a `v*` tag runs GoReleaser, which builds every published OS/arch, writes the
checksum file and registry manifest, and signs the checksums with the release GPG key
(repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`). Both registries require signed
releases.

Semver for providers: removing or renaming a resource or attribute, changing an attribute's
type incompatibly, or adding a default that does not match the API's own default are all
breaking changes and need a major bump. Deprecating something is a minor bump — deprecate
first, remove in the next major.
