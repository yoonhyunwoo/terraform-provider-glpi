terraform {
  required_providers {
    glpi = {
      source = "yoonhyunwoo/glpi"
    }
  }
}

# url, user_token and app_token can also come from GLPI_URL, GLPI_USER_TOKEN
# and GLPI_APP_TOKEN.
provider "glpi" {
  url        = "https://glpi.example.com/apirest.php"
  user_token = var.glpi_user_token
}

variable "glpi_user_token" {
  type      = string
  sensitive = true
}
