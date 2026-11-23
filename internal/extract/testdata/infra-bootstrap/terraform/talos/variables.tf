variable "nodes" {
  description = "Control-plane nodes: name => static IP."
  type        = map(string)
  default = {
    esprimo-1  = "192.168.1.202"
    esprimo-2  = "192.168.1.203"
    talos-cp-3 = "192.168.1.201"
  }
}

variable "serials" {
  type = map(string)
}
