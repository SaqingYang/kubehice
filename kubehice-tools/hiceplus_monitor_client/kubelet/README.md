# 使用方法
## 构建
```bash
# 对应于KubeEdge Edgecore
docker build -t local-registry:5000/hice-cli:latest-edge -f Dockerfile.edge .
docker build -t local-registry:5000/hice-cli:latest-edge-arm64 -f Dockerfile.edge .

# 对应于Kubelet
docker build -t local-registry:5000/hice-cli:latest-cloud .
docker build -t local-registry:5000/hice-cli:latest-cloud-arm64 .
```
## 启动
```bash
# 对应于Kubelet
bash -c "/root/cli --node=cloud-worker1 --cloud=true --url=https://192.168.10.2:10250/metrics/cadvisor --server=192.168.10.1:12345"

# 对应于KubeEdge Edgecore
bash -c "--node=edge1 --cloud=false --url=https://192.168.10.2:10350/metrics/cadvisor --server=192.168.10.1:12345"
```