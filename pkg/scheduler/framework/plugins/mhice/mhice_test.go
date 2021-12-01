package mhice

import (
	"encoding/json"
	"testing"

	v1 "k8s.io/api/core/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestGetExistedNeighborEdge(t *testing.T) {
	// 使用Label指明Service对应的Pod，此Label在Pod中存在
	existedPods := &v1.PodList{}
	yamlPod := `
apiVersion: v1
kind: Pod
metadata:
  name: nginx-hc-pod
  labels: 
    app: nginx-hc
spec:
  schedulerName: kubehice-scheduler
  containers:
  - name: nginx
    image: nginx1:latest
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: 100m
      limits:
        cpu: 200m
  - name: nginx
    image: local-registry:5000/redis:latest
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: 100m
      limits:
        cpu: 200m
`
	jsonPod, _ := k8sYaml.ToJSON([]byte(yamlPod))
	p1 := &v1.Pod{}
	json.Unmarshal([]byte(jsonPod), p1)
	p2 := p1.DeepCopy()
	p3 := p1.DeepCopy()
	p1.Spec.NodeName = "n1"
	p1.Labels["app"] = "svc1"
	p2.Spec.NodeName = "n2"
	p2.Labels["app"] = "svc2"
	p3.Spec.NodeName = "n1"
	p3.Labels["app"] = "redis-cart"
	existedPods.Items = append(existedPods.Items, *p1, *p2, *p3)

	graph := GenerateServiceGraph()
	currentSvc := "frontend"
	pIndex := graph.Vertex[currentSvc]

	sum := int64(0)
	for k, v := range graph.Vertex {
		if graph.Edge[v][pIndex] >= 0 {
			t.Log(currentSvc+" <- ", graph.Edge[v][pIndex], "--", k)
			sum += graph.Edge[v][pIndex]
		}
		if graph.Edge[pIndex][v] >= 0 {
			t.Log(currentSvc+" -- ", graph.Edge[pIndex][v], "->", k)
			sum += graph.Edge[pIndex][v]
		}
	}

	for _, p := range existedPods.Items {
		val, ok := graph.Vertex[p.Labels["app"]]
		if ok {
			if graph.Edge[val][pIndex] >= 0 {
				t.Log(currentSvc+" <- ", graph.Edge[val][pIndex], "--", p.Labels["app"])
				sum += graph.Edge[val][pIndex]
			}
			if graph.Edge[pIndex][val] >= 0 {
				t.Log(currentSvc+" -- ", graph.Edge[pIndex][val], "->", p.Labels["app"])
				sum += graph.Edge[pIndex][val]
			}
		}
	}
	t.Log("neighbor sum = ", sum)

}
