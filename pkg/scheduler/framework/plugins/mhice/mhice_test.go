package mhice

import "testing"

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
}
