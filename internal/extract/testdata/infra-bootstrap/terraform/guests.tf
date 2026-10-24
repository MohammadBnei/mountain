variable "pve_node_name" { type = string }

resource "proxmox_virtual_environment_vm" "pg02" {
  name      = "pg02"
  node_name = var.pve_node_name
}

resource "proxmox_virtual_environment_container" "build_runner" {
  node_name = "server1"
  initialization {
    hostname = "build-runner"
  }
}

resource "proxmox_virtual_environment_container" "k9s_dashboard" {
  node_name = "server1"
  initialization {
    hostname = "k9s-dashboard"
  }
}

resource "proxmox_virtual_environment_vm" "template" {
  name      = "ubuntu-template"
  node_name = var.pve_node_name
  template  = true
}

resource "proxmox_virtual_environment_vm" "k8s_node" {
  for_each  = var.k8s_nodes
  name      = each.key
  node_name = coalesce(each.value.node_name, var.pve_node_name)
}

resource "proxmox_virtual_environment_vm" "mystery" {
  name      = "mystery"
  node_name = local.somewhere
}

resource "proxmox_virtual_environment_file" "not_a_guest" {
  node_name = "server1"
}
