terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

provider "docker" {
  host = "unix:///var/run/docker.sock"
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU"
  description  = "Persistent parameter used to validate the proxy policy."
  type         = "number"
  default      = "2"
  mutable      = true

  option {
    name  = "2 cores"
    value = "2"
  }
  option {
    name  = "4 cores"
    value = "4"
  }
}

data "coder_parameter" "debug" {
  name         = "debug"
  display_name = "Debug for one build"
  description  = "Ephemeral parameter used to validate that coder-rbac rejects value changes."
  type         = "bool"
  default      = "false"
  mutable      = true
  ephemeral    = true
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  startup_script = <<-EOT
    mkdir -p /tmp/coder-rbac-app
    printf '%s\n' '<!doctype html><title>coder-rbac wildcard</title><h1>Wildcard application reached through coder-rbac</h1>' > /tmp/coder-rbac-app/index.html
    exec python3 -m http.server 13337 --directory /tmp/coder-rbac-app
  EOT

  env = {
    CODER_URL           = "http://coder-rbac:8080"
    CODER_SESSION_TOKEN = data.coder_workspace_owner.me.session_token
    DEMO_CPU            = data.coder_parameter.cpu.value
    DEMO_DEBUG          = data.coder_parameter.debug.value
  }
}

resource "coder_app" "wildcard" {
  agent_id     = coder_agent.main.id
  slug         = "wildcard"
  display_name = "Wildcard validation"
  url          = "http://localhost:13337"
  subdomain    = true
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337"
    interval  = 5
    threshold = 6
  }
}

resource "docker_volume" "home" {
  name = "coder-${data.coder_workspace.me.id}-home"

  lifecycle {
    ignore_changes = all
  }

  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-base:ubuntu"
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name

  entrypoint = [
    "sh",
    "-c",
    replace(coder_agent.main.init_script, "http://coder.localhost:7080", "http://coder-rbac:8080"),
  ]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_URL=http://coder-rbac:8080",
    "CODER_SESSION_TOKEN=${data.coder_workspace_owner.me.session_token}",
    "DEMO_CPU=${data.coder_parameter.cpu.value}",
    "DEMO_DEBUG=${data.coder_parameter.debug.value}",
  ]

  networks_advanced {
    name = "coder-rbac-workspace"
  }

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home.name
    read_only      = false
  }

  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
