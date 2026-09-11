terraform {
  required_providers {
    omada = {
      source  = "filipegalo/omada"
      version = "~> 0.1"
    }
  }
}

provider "omada" {
  # Every attribute can also come from an environment variable, which is the
  # recommended way to keep credentials out of configuration: OMADA_URL,
  # OMADA_USERNAME, OMADA_PASSWORD, OMADA_SITE, OMADA_SKIP_TLS_VERIFY.
  # username and password are omitted here and read from the environment.
  base_url = "https://192.168.1.1:8043"
  site     = "Default"

  # Controllers commonly serve a self-signed certificate on their local port.
  skip_tls_verify = true
}
