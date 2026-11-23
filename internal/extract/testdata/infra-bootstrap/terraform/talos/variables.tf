variable "nodes" {
  description = "Control-plane nodes: name => static IP."
  type        = map(string)
  default = {
    esprimo-1  = "192.168.1.208"
    esprimo-2  = "192.168.1.209"
    talos-cp-3 = "192.168.1.210"
  }
}

variable "serials" {
  type = map(string)
}
