terraform {
  required_providers {
    omada = {
      source  = "filipegalo/omada"
      version = "~> 0.3"
    }
  }
}

provider "omada" {
  # Every attribute can also come from an environment variable, which is the
  # recommended way to keep credentials out of configuration: OMADA_URL,
  # OMADA_USERNAME, OMADA_PASSWORD, OMADA_CLIENT_ID, OMADA_CLIENT_SECRET,
  # OMADA_SITE, OMADA_SKIP_TLS_VERIFY.
  # username and password are omitted here and read from the environment.
  base_url = "https://192.168.1.1:8043"
  site     = "Default"

  # Optional. When configured, public Open API requests use a scoped
  # application token. Internal web API requests keep using the classic
  # session. Without these fields every request uses the classic session.
  client_id     = "terraform"
  client_secret = "replace-with-the-open-api-app-secret"

  # Controllers commonly serve a self-signed certificate on their local port.
  skip_tls_verify = true
}
