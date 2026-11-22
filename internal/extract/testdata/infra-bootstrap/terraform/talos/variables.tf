variable "nodes" {
  description = "Control-plane nodes: name => static IP."
  type        = map(string)
  default = {
    k8s-cp-01     = "192.168.1.201"
    k8s-worker-01 = "192.168.1.202"
  }
}

variable "serials" {
  type = map(string)
}
