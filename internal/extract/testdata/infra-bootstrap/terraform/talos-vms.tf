# A Talos node that is also a Proxmox VM (infra-bootstrap terraform/talos-vms.tf):
# the machines extractor must read it once, as the k8s node.
resource "proxmox_virtual_environment_vm" "talos_cp_3" {
  name      = "talos-cp-3"
  node_name = "server1"
  vm_id     = 210
}
