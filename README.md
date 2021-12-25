# KubeHICE
## Hicev1
### 简介
在Kubernetes 1.22 调度器基础上，实现面向ISA异构环境的容器部署扩展能力，实现像操作同构集群一样在ISA异构集群上部署容器化应用。
### 部署方式
#### 编译
```bash
cd cmd/kube-scheduler
go build .
```
#### 运行
##### 调度器配置文件
```yaml
# 调度器配置文件scheduler-configv1.yml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
clientConnection:
        kubeconfig: ./scheduler.conf # 就是master节点上/etc/kubernetes/scheduler.conf
leaderElection:
        leaderElect: false
profiles:
  - schedulerName: kubehice-scheduler # 与编排脚本中的schedulerName对应
    plugins:
      preFilter:
        enabled:
          - name: Hicev1
      filter:
        enabled:
          - name: Hicev1
      bind:
        enabled:
          - name: Hicev1
        disabled:
          - name: DefaultBinder
```
##### 运行
```bash
# ETCD_SERVER是Etcd运行的节点，PEER_KEY、PEER_CRT、CA_CRT是Etcd的访问密钥，从Master节点的/etc/kubernetes/pki下获得，address是API Server所在节点
ETCD_SERVER=192.168.10.2:2379 PEER_KEY=peer.key PEER_CRT=peer.crt CA_CRT=ca.crt ./kube-scheduler  --config scheduler-configv1.yml --address=192.168.10.2
```
##### 手动导入多架构镜像信息
```bash
# 在/kubehice-tools/multi-arch-images目录下，images.yaml是多架构镜像信息文件
./multi-arch-images
```
##### 自动识别多架构镜像信息
```bash
# 在/kubehice-tools/container-manifests目录下，需要Docker CLI支持，同时在/etc/docker/daemon.json文件中添加"experimental":true
./container-manifests
# 注意：1分钟查询一次镜像索引，若获取失败会Sleep 1 minute，如果镜像未使用镜像索引，则查到的manifests文件中不会有任何平台架构信息，则查找镜像名中是否包含 amd64、arm64等字段，目前只考虑了Linux系统，如果没有，则默认此镜像只支持amd64（x86-64）
```
```/etc/docker/daemon.json```示例
```json
{
  "exec-opts": ["native.cgroupdriver=systemd"],
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m"
  },
  "storage-driver": "overlay2",
  "insecure-registries": ["local-registry:5000"],
  "experimental":true
}
```
##### 使用
编排脚本示例
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: nginx
  name: nginx-deployment
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 2
  template:
    metadata:
      labels:
        app: nginx
    spec:
      schedulerName: kubehice-scheduler
      containers:
      - image: local-registry:5000/test-nginx:latest # local-registry是一个本地镜像仓库
        name: nginx
        imagePullPolicy: Always
        ports:
        - containerPort: 80
          hostPort: 80
        resources:
          requests:
            cpu: 100m
          limits:
            cpu: 100m
```
多架构镜像信息示例
```yaml
list:
  - name: local-registry:5000/test-nginx:latest
    images:
    - name: local-registry:5000/test-nginx:latest-amd64
      arch: amd64
      os: linux
    - name: local-registry:5000/test-nginx:latest-arm64
      arch: arm64
      os: linux
```
若Deployment的一个副本分配到了amd64节点，将根据```local-registry:5000/test-nginx:latest-amd64```拉取镜像，若分配到了arm64节点，将根据```local-registry:5000/test-nginx:latest-arm64```拉取镜像，Hicev1会避免将Pod分配到amd64、arm64架构之外的节点上。
## Hicev2
### 简介
节点能力感知的资源分配方案和调度方案，目前云、边集群中各节点存在单线程能力异构的现象，给定资源配置下，一个容器分配到不同的节点会有不同的性能表现，考虑当前流行的微服务架构下单个微服务所需资源不高（如小于1CPU），容器也具有细粒度的CPU资源控制能力，开发了Hicev2，Hicev2重新量化了CPU资源，添加了一个节点单线程能力系数“hice.kj”，表示节点的单线程能力差异，如节点1的hice.kj=1，节点2的hice.kj=0.5，表示节点1的单线程能力是节点2的2倍。

假设我们的部署场景：资源配置的数值按照一个参考节点确定，如测试节点，假设它的hice.kj=1，CPU资源配置为
```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 200m
    memory: 256Mi
```
集群中存在单线程能力不同的节点，假设有一个hice.kj=0.5的节点，当此容器分配到此节点上时，Hicev2将给它分配资源如下
```yaml
resources:
  requests:
    cpu: 200m
    memory: 128Mi
  limits:
    cpu: 400m
    memory: 256Mi
```
这样无论此容器在原始配置（参考节点）下，还是在hice.kj=0.5的节点上运行，都会表现相近的性能。

多副本应用场景，Kubernetes Service、服务网格Istio等，大多数应用场景下，它们会将服务请求均匀的转发到关联的Endpoints上，虽然有不同的转发策略，但从统计角度看，请求转发都是均匀的，其它场景如设置```internalTrafficPolicy=Local```或把注解```service.kubernetes.io/topology-aware-hints``` 的值设置为 ```auto``除外。

在均匀转发策略下，一个Service的不同Endpoints会获得相当大小的负载，由于节点单线程能力的异构，不同的副本将表现出不同的CPU使用率，当压力增加时，低性能节点上的副本首先会满载，与此同时，位于高性能节点上的副本负载依然较低，低性能节点成为此Service的瓶颈，给高性能节点上分配的CPU资源将永远无法充分利用。

节点可承载应用的数量考虑，假设节点1和节点2都有2CPU/vCPU，但节点1的单线程能力是节点2的2倍，Kubernetes不会感知到它们的差异，将会给它们相同数量的负载，可以在Kubelet启动参数中进行限制，但很明显，由于负载的多样性，这种方式并不合理。Hicev2的资源分配策略下，保证节点上所有Pod的```requests.cpu```之和不会超过节点可分配CPU资源数量，可以根据节点的计算能力来控制节点承载的容器数量。

### 部署
#### 编译
与Hicev1相同
#### 运行
##### 调度器配置文件
```yaml
# scheduler-configv2.yml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
clientConnection:
        kubeconfig: ./scheduler.conf
leaderElection:
        leaderElect: false
profiles:
  - schedulerName: kubehice-scheduler
    plugins:
      preFilter:
        enabled:
          - name: Hicev1
#          - name: Hicev2
      filter:
        enabled:
          - name: Hicev1
          - name: Hicev2
        disabled:
          - name: NodeResourcesFit
      score:
         disabled: #下面的组件与Hicev2的CPU资源量化方式有冲突，因此禁用它们
           - name: NodeResourcesLeastAllocated
           - name: NodeResourcesBalancedAllocation
           - name: NodeResourcesMostAllocated
           - name: RequestedToCapacityRatio  
         enabled:
           - name: Hicev2
      bind:
        enabled:
          - name: Hicev2
        disabled:
          - name: DefaultBinder
```
##### 运行
```bash
# ETCD_SERVER是Etcd运行的节点，PEER_KEY、PEER_CRT、CA_CRT是Etcd的访问密钥，从Master节点的/etc/kubernetes/pki下获得，address是API Server所在节点
ETCD_SERVER=192.168.10.2:2379 PEER_KEY=peer.key PEER_CRT=peer.crt CA_CRT=ca.crt ./kube-scheduler  --config scheduler-configv2.yml --address=192.168.10.2
```
##### 标记节点单线程能力
```bash
kubectl label node node1 hice.kj=1.0
kubectl label node node2 hice.kj=0.8
kubectl label node node3 hice.kj=1.2
```

##### 编排脚本示例
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: nginx
  name: nginx-deployment
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 2
  template:
    metadata:
      labels:
        app: nginx
        hice.kb: "0.8"
    spec:
      schedulerName: kubehice-scheduler
      containers:
      - image: local-registry:5000/test-nginx:latest 
        name: nginx
        imagePullPolicy: Always
        resources:
          requests:
            cpu: 100m
          limits:
            cpu: 100m
---
kind: Service
apiVersion: v1
metadata:
  name: test-nginx-svc
spec:
  ports:
    - name: http-0
      port: 12345
      targetPort: 80
      protocol: TCP
  selector:
    app: nginx
```
假设副本1分配到了node1，副本2分配到了node2，副本1将分配125m CPU，副本2将分配100m CPU，通过压力测试工具，向test-nginx-svc发请求，Deployment的两个副本的CPU资源使用率将接近100%，即副本1 nginx进程12.5%， 副本2 nginx进程10%。

##### 节点单线程能力估计
根据Service关联的多副本的CPU使用率评估节点的单线程能力，位于/kubehice-tools/analyse、/kubehice-tools/hiceplus_monitor_client、/kubehice-tools/hiceplus_monitor_server和一个外部数据库，目前并不完善。