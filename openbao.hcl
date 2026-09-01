ui = true


audit "file" "file" {
  description = "Audit file for POC"
  options = {
    file_path = "/tmp/openbao.log"
    log_raw   = "true"
  }
}

audit "http" "http_audit_device_sync" {
  description = "Audit http listener: Synchronizer"
  options {
    uri     = "http://controller:8080/ingress"
    log_raw = "true"

    headers = <<EOT
{"X-OpenBao-Audit-Token":["dev-audit-token"]}
EOT
  }
}
