package mhice

import (
	"encoding/json"
	"testing"

	v1 "k8s.io/api/core/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
)

func GenerateServiceGraph() *MicroServiceGraph {
	graph := &MicroServiceGraph{Num: 3}
	edges := [][]int64{{-1, 100, -1}, {-1, -1, 100}, {-1, -1, -1}}
	graph.Vertex = make(map[string]int)
	graph.Vertex["svc1"] = 0
	graph.Vertex["svc2"] = 1
	graph.Vertex["svc3"] = 2
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			graph.Edge[i][j] = edges[i][j]
		}
	}
	return graph
}
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
	p3.Labels["app"] = "svc2"
	existedPods.Items = append(existedPods.Items, *p1, *p2, *p3)
	t.Log(existedPods.Items[0].Labels)
	t.Log(existedPods.Items[1].Labels)
	t.Log(existedPods.Items[2].Labels)

	graph := GenerateServiceGraph()
	pIndex := graph.Vertex["svc3"]

	for i := 0; i < graph.Num; i++ {
		if graph.Edge[i][pIndex] >= 0 {
			t.Log("svc3 <- ", i)
		}
		if graph.Edge[pIndex][i] >= 0 {
			t.Log("svc3 ->", i)
		}
	}

}
