sudo sysctl -w fs.inotify.max_user_watches=2099999999
sudo sysctl -w fs.inotify.max_user_instances=2099999999
sudo sysctl -w fs.inotify.max_queued_events=2099999999


bash hack/local-up-karmada.sh

export KUBECONFIG=${HOME}/.kube/karmada.config
kubectl config use-context karmada-host

kubectl apply -f artifacts/dashboard/karmada-dashboard-sa.yaml
kubectl apply -f artifacts/dashboard/karmada-dashboard-api.yaml
kubectl apply -f artifacts/dashboard/karmada-dashboard-web.yaml
kubectl apply -f artifacts/dashboard/karmada-dashboard-configmap.yaml

make karmada-dashboard-api

export KUBECONFIG=${HOME}/.kube/karmada.config
kubectl config use-context karmada-apiserver
kubectl apply -f artifacts/dashboard/karmada-dashboard-sa.yaml
kubectl -n karmada-system get secret/karmada-dashboard-secret -ojsonpath='{.data.token}' | base64 -d && echo


export KUBECONFIG=${HOME}/.kube/members.config
kubectl config use-context member3
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' member3-control-plane)

kubectl --kubeconfig ~/.kube/karmada.config --context karmada-apiserver patch cluster member3 --type='merge' -p "{\"spec\":{\"apiEndpoint\":\"https://$IP:6443\"}}"

export KUBECONFIG=${HOME}/.kube/karmada.config
_output/bin/linux/amd64/karmada-dashboard-api \
  --karmada-kubeconfig=/home/asif/.kube/karmada.config \
  --karmada-context=karmada-apiserver \
  --skip-karmada-apiserver-tls-verify \
  --kubeconfig=/home/asif/.kube/karmada.config \
  --context=karmada-host \
  --insecure-port=8000
  
  
  
