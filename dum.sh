make karmada-dashboard-api

export KUBECONFIG=${HOME}/.kube/karmada.config
_output/bin/linux/amd64/karmada-dashboard-api \
  --karmada-kubeconfig=/home/ubuntu/.kube/karmada.config \
  --karmada-context=karmada-apiserver \
  --skip-karmada-apiserver-tls-verify \
  --kubeconfig=/home/ubuntu/.kube/karmada.config \
  --context=karmada-host \
  --insecure-port=8000
